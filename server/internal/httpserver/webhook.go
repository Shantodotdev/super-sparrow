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

const maxWebhookBytes = 10 << 20

// githubWebhook closes the linked issue when a pull request merges. Every delivery must carry a
// valid X-Hub-Signature-256 for the shared secret; anything else is refused before it is read
// as JSON. The change is recorded as the system actor, since no person or agent made it.
func (s *server) githubWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		return &store.ValidationError{Field: "body", Message: fmt.Sprintf("must be at most %d bytes", maxWebhookBytes)}
	}
	if !validSignature(s.webhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: "the X-Hub-Signature-256 header does not match the webhook secret"})
		return nil
	}

	switch r.Header.Get("X-GitHub-Event") {
	case "ping":
		writeJSON(w, http.StatusOK, map[string]string{"result": "pong"})
		return nil
	case "pull_request":
	default:
		writeJSON(w, http.StatusAccepted, map[string]string{"result": "ignored"})
		return nil
	}

	var event struct {
		Action      string `json:"action"`
		PullRequest struct {
			HTMLURL string `json:"html_url"`
			Merged  bool   `json:"merged"`
			Head    struct {
				Ref string `json:"ref"`
			} `json:"head"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return &store.ValidationError{Field: "body", Message: "must be a pull_request event"}
	}
	if event.Action != "closed" || !event.PullRequest.Merged {
		writeJSON(w, http.StatusAccepted, map[string]string{"result": "ignored"})
		return nil
	}

	issue, changed, err := s.store.CloseIssueForMergedPullRequest(r.Context(), store.Caller{ActorID: store.SystemActorID},
		store.MergedPullRequest{URL: event.PullRequest.HTMLURL, Branch: event.PullRequest.Head.Ref})
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

// validSignature checks GitHub's sha256=<hex HMAC of the body> signature in constant time.
func validSignature(secret string, body []byte, header string) bool {
	given, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	decoded, err := hex.DecodeString(given)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(decoded, mac.Sum(nil))
}
