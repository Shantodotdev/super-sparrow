package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// maxWebhookBytes bounds a delivery. Pull request events are tens of kilobytes; the limit leaves
// room for large ones without letting unsigned requests make the server buffer much.
const maxWebhookBytes = 1 << 20

// maxDeliveryIDLength bounds the X-GitHub-Delivery header, which names a delivery in the store.
const maxDeliveryIDLength = 100

const signatureMismatch = "the X-Hub-Signature-256 header does not match the webhook secret"

// webhookEvents are the GitHub events the server acts on. Any other is acknowledged and dropped.
var webhookEvents = map[string]bool{
	"ping": true, "installation": true, "installation_repositories": true, "pull_request": true,
	"pull_request_review": true, "pull_request_review_comment": true, "issue_comment": true,
	"check_suite": true, "check_run": true, "status": true,
}

// githubWebhook passes GitHub's deliveries to the GitHub service, which records pull requests,
// reviews, comments, and checks. Every delivery must carry a valid X-Hub-Signature-256 for the
// shared secret; anything else is refused before it is read as JSON. A missing or malformed
// signature, or an event the server has no use for, is answered before the body is read at all.
// The changes are recorded as the system actor, since no person or agent made them.
func (s *server) githubWebhook(w http.ResponseWriter, r *http.Request) error {
	signature, ok := parseSignature(r.Header.Get("X-Hub-Signature-256"))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: signatureMismatch})
		return nil
	}

	event := r.Header.Get("X-GitHub-Event")
	if !webhookEvents[event] {
		writeJSON(w, http.StatusAccepted, map[string]string{"result": "ignored"})
		return nil
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" || len(deliveryID) > maxDeliveryIDLength {
		return &store.ValidationError{Field: "X-GitHub-Delivery", Message: fmt.Sprintf("must be a delivery ID of at most %d characters", maxDeliveryIDLength)}
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

	result, err := s.github.HandleDelivery(r.Context(), event, deliveryID, body)
	if err != nil {
		return err
	}
	if result == "pong" {
		writeJSON(w, http.StatusOK, map[string]string{"result": result})
		return nil
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"result": result})
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
