// Package email sends the server's emails: sign-in codes and invitations.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const resendEndpoint = "https://api.resend.com/emails"

// Message is an email to one recipient, with an HTML body and a plain-text one for clients
// that do not show HTML.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Sender delivers emails.
type Sender interface {
	Send(ctx context.Context, message Message) error
}

// Resend sends emails through the Resend API.
type Resend struct {
	apiKey   string
	from     string
	endpoint string
	client   *http.Client
}

// NewResend returns a Sender that sends every email from from, authenticated with apiKey.
func NewResend(apiKey, from string) *Resend {
	return &Resend{
		apiKey:   apiKey,
		from:     from,
		endpoint: resendEndpoint,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text"`
}

// Send delivers message. It fails when Resend does not accept the email, with Resend's status
// and the start of its response, which never contains the API key.
func (r *Resend) Send(ctx context.Context, message Message) error {
	body, err := json.Marshal(resendRequest{From: r.from, To: []string{message.To}, Subject: message.Subject, HTML: message.HTML, Text: message.Text})
	if err != nil {
		return fmt.Errorf("encode email: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build email request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+r.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("send email through Resend: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("send email through Resend: status %d: %s", response.StatusCode, bytes.TrimSpace(detail))
	}

	return nil
}
