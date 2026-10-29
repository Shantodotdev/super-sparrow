package httpserver

import (
	"errors"
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// codeSentMessage is the answer to every valid code request, so it never reveals who has an
// account.
const codeSentMessage = "If this email belongs to a teammate, a sign-in code is on its way."

func (s *server) requestCode(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	if err := s.auth.RequestCode(r.Context(), body.Email, clientIP(r, s.trustedProxies)); err != nil {
		return err
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"message": codeSentMessage})
	return nil
}

func (s *server) verifyCode(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	signIn, err := s.auth.VerifyCode(r.Context(), body.Email, body.Code, deviceLabel(r.UserAgent()))
	if err != nil {
		return err
	}

	s.setSessionCookie(w, signIn.Secret, signIn.Session.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"actor": newActorView(signIn.Actor)})
	return nil
}

func (s *server) signOut(w http.ResponseWriter, r *http.Request) error {
	id := identityFrom(r.Context())
	if id.sessionID == "" {
		return forbiddenRequest("sign-out ends a browser session; revoke a token instead")
	}

	if err := s.store.RevokeSession(r.Context(), id.caller(), id.sessionID); err != nil {
		return err
	}

	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) me(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, map[string]any{"actor": newActorView(identityFrom(r.Context()).actor)})
	return nil
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) error {
	id := identityFrom(r.Context())
	sessions, err := s.store.ListSessions(r.Context(), id.actor.ID)
	if err != nil {
		return err
	}

	views := make([]sessionView, 0, len(sessions))
	for _, session := range sessions {
		views = append(views, newSessionView(session, id.sessionID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": views})
	return nil
}

func (s *server) revokeSession(w http.ResponseWriter, r *http.Request) error {
	id := identityFrom(r.Context())
	if err := s.store.RevokeSession(r.Context(), id.caller(), r.PathValue("id")); err != nil {
		return err
	}

	if r.PathValue("id") == id.sessionID {
		s.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) revokeAllSessions(w http.ResponseWriter, r *http.Request) error {
	count, err := s.store.RevokeAllSessions(r.Context(), identityFrom(r.Context()).caller())
	if err != nil {
		return err
	}

	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]int{"revoked": count})
	return nil
}

// listPeople returns every actor. Agents see names but not email addresses, which they have no
// use for.
func (s *server) listPeople(w http.ResponseWriter, r *http.Request) error {
	actors, err := s.store.ListActors(r.Context())
	if err != nil {
		return err
	}

	hideEmails := identityFrom(r.Context()).actor.Kind == store.ActorAgent
	views := make([]actorView, 0, len(actors))
	for _, actor := range actors {
		view := newActorView(actor)
		if hideEmails {
			view.Email = ""
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"actors": views})
	return nil
}

func (s *server) invitePerson(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	actor, err := s.auth.Invite(r.Context(), identityFrom(r.Context()).caller(),
		store.NewHuman{DisplayName: body.DisplayName, Email: body.Email, Role: store.Role(body.Role)})
	sent := err == nil
	if errors.Is(err, auth.ErrInvitationNotSent) {
		s.logger.ErrorContext(r.Context(), "invitation email failed", "actor_id", actor.ID, "error", err)
	} else if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"actor": newActorView(actor), "invitation_sent": sent})
	return nil
}

func (s *server) disablePerson(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.DisableActor(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id")); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// listTokens returns the caller's tokens. An owner can ask for everyone's with ?all=true, so they
// can revoke any of them.
func (s *server) listTokens(w http.ResponseWriter, r *http.Request) error {
	id := identityFrom(r.Context())
	owner := id.actor.ID
	switch r.URL.Query().Get("all") {
	case "":
	case "true":
		if !id.actor.IsOwner() {
			return forbiddenRequest("only an owner can list everyone's tokens")
		}
		owner = ""
	default:
		return &store.ValidationError{Field: "all", Message: "must be true when given"}
	}

	tokens, err := s.store.ListAgentTokens(r.Context(), owner)
	if err != nil {
		return err
	}

	views := make([]tokenView, 0, len(tokens))
	for _, token := range tokens {
		views = append(views, newTokenView(token))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": views})
	return nil
}

func (s *server) createToken(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		AgentName string `json:"agent_name"`
		Label     string `json:"label"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	created, err := s.auth.CreateToken(r.Context(), identityFrom(r.Context()).caller(), body.AgentName, body.Label)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"token": newTokenView(created.Token), "secret": created.Secret})
	return nil
}

func (s *server) renewToken(w http.ResponseWriter, r *http.Request) error {
	token, err := s.store.RenewAgentToken(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id"))
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"token": newTokenView(token)})
	return nil
}

func (s *server) revokeToken(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.RevokeAgentToken(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id")); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
