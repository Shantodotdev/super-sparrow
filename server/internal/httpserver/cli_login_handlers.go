package httpserver

import (
	"net/http"
	"net/url"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	// cliLoginPollInterval is how long a CLI waits between checks on its sign-in request.
	cliLoginPollInterval = 2 * time.Second
	// approveLinkBase opens the desktop app's approval screen for a code.
	approveLinkBase = "super-sparrow://cli-login"
)

type cliLoginView struct {
	DeviceLabel   string    `json:"device_label"`
	RequestedFrom string    `json:"requested_from"`
	SameAddress   bool      `json:"same_address"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// startCLILogin begins a CLI sign-in that a person approves in the desktop app. The device code
// in the answer is the CLI's secret for polling; the user code is safe to show.
func (s *server) startCLILogin(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		DeviceLabel string `json:"device_label"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	started, err := s.auth.StartCLILogin(r.Context(), body.DeviceLabel, clientIP(r, s.trustedProxies))
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":      started.DeviceCode,
		"user_code":        started.UserCode,
		"expires_at":       started.ExpiresAt,
		"interval_seconds": int(cliLoginPollInterval / time.Second),
		"approve_url":      approveLinkBase + "?" + url.Values{"code": {started.UserCode}}.Encode(),
	})
	return nil
}

// pollCLILogin tells a CLI where its sign-in stands. The one answer with status claimed carries
// the CLI's session token, shown this once.
func (s *server) pollCLILogin(w http.ResponseWriter, r *http.Request) error {
	if ok, retry := s.perPollIP.Allow(clientIP(r, s.trustedProxies)); !ok {
		return &auth.RateLimitError{RetryAfter: retry}
	}
	var body struct {
		DeviceCode string `json:"device_code"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if body.DeviceCode == "" {
		return &store.ValidationError{Field: "device_code", Message: "is required"}
	}

	claim, err := s.auth.ClaimCLILogin(r.Context(), body.DeviceCode)
	if err != nil {
		return err
	}

	if claim.Status != store.CLILoginClaimed {
		writeJSON(w, http.StatusOK, map[string]any{"status": claim.Status})
		return nil
	}
	signIn := claim.SignIn
	writeJSON(w, http.StatusOK, map[string]any{
		"status": claim.Status, "token": signIn.Secret,
		"session": newSessionView(signIn.Session, signIn.Session.ID), "actor": newActorView(signIn.Actor),
	})
	return nil
}

// getCLILogin shows a pending CLI sign-in to the person about to approve it, with whether it came
// from the same address they are on, so a request someone else started stands out.
func (s *server) getCLILogin(w http.ResponseWriter, r *http.Request) error {
	if ok, retry := s.perPollIP.Allow(clientIP(r, s.trustedProxies)); !ok {
		return &auth.RateLimitError{RetryAfter: retry}
	}
	login, err := s.auth.FindCLILogin(r.Context(), r.PathValue("code"))
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, s.newCLILoginView(r, login))
	return nil
}

func (s *server) approveCLILogin(w http.ResponseWriter, r *http.Request) error {
	return s.decideCLILogin(w, r, true)
}

func (s *server) denyCLILogin(w http.ResponseWriter, r *http.Request) error {
	return s.decideCLILogin(w, r, false)
}

// decideCLILogin approves or denies a CLI sign-in as the person in the desktop app. The store
// refuses CLI sessions and agents, so a CLI cannot sign in more CLIs.
func (s *server) decideCLILogin(w http.ResponseWriter, r *http.Request, approve bool) error {
	login, err := s.auth.DecideCLILogin(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("code"), approve)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": login.Status})
	return nil
}

func (s *server) newCLILoginView(r *http.Request, login store.CLILogin) cliLoginView {
	return cliLoginView{
		DeviceLabel: login.DeviceLabel, RequestedFrom: login.RequestedFrom,
		SameAddress: login.RequestedFrom == clientIP(r, s.trustedProxies),
		ExpiresAt:   login.ExpiresAt, CreatedAt: login.CreatedAt,
	}
}
