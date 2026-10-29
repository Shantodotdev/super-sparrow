package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// maxWebhookBytes bounds a delivery. Pull request events are tens of kilobytes; the limit leaves
// room for large ones without letting unsigned requests make the server buffer much.
const maxWebhookBytes = 1 << 20

const signatureMismatch = "the X-Hub-Signature-256 header does not match the webhook secret"

// githubWebhook closes the linked issue when a pull request merges. Every delivery must carry a
// valid X-Hub-Signature-256 for the shared secret; anything else is refused before it is read
// as JSON. A missing or malformed signature, or an event the server has no use for, is answered
// before the body is read at all. The change is recorded as the system actor, since no person or
// agent made it.
func (s *server) githubWebhook(w http.ResponseWriter, r *http.Request) error {
	signature, ok := parseSignature(r.Header.Get("X-Hub-Signature-256"))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: signatureMismatch})
		return nil
	}

	event := r.Header.Get("X-GitHub-Event")
	if event != "ping" && event != "pull_request" {
		writeJSON(w, http.StatusAccepted, map[string]string{"result": "ignored"})
		return nil
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		return &store.ValidationError{Field: "body", Message: fmt.Sprintf("must be at most %d bytes", maxWebhookBytes)}
	}
	mac := hmac.New(sha256.New, []byte(s.webhookSecret))
	mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: signatureMismatch})
		return nil
	}

	if event == "ping" {
		writeJSON(w, http.StatusOK, map[string]string{"result": "pong"})
		return nil
	}

	var delivery struct {
		Action      string `json:"action"`
		PullRequest struct {
			HTMLURL string `json:"html_url"`
			Merged  bool   `json:"merged"`
			Head    struct {
				Ref string `json:"ref"`
			} `json:"head"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &delivery); err != nil {
		return &store.ValidationError{Field: "body", Message: "must be a pull_request event"}
	}
	if delivery.Action != "closed" || !delivery.PullRequest.Merged {
		writeJSON(w, http.StatusAccepted, map[string]string{"result": "ignored"})
		return nil
	}

	issue, changed, err := s.store.CloseIssueForMergedPullRequest(r.Context(), store.Caller{ActorID: store.SystemActorID},
		store.MergedPullRequest{URL: delivery.PullRequest.HTMLURL, Branch: delivery.PullRequest.Head.Ref})
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusAccepted, map[string]string{"result": "no matching issue"})
		return nil
	}
	if err != nil {
		return err
	}

	result := "already closed"
	if changed {
		result = "closed"
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": result, "issue": issue.Key()})
	return nil
}

// parseSignature reads GitHub's sha256=<hex HMAC of the body> signature header.
func parseSignature(header string) ([]byte, bool) {
	given, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return nil, false
	}
	decoded, err := hex.DecodeString(given)
	if err != nil || len(decoded) != sha256.Size {
		return nil, false
	}
	return decoded, true
}
