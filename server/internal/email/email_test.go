package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendSendsTheMessage(t *testing.T) {
	var got resendRequest
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Write([]byte(`{"id":"email-1"}`))
	}))
	defer server.Close()

	sender := NewResend("re_test", "Super Sparrow <sparrow@example.com>")
	sender.endpoint = server.URL

	err := sender.Send(context.Background(), Message{To: "max@example.com", Subject: "Hello", HTML: "<p>Body</p>", Text: "Body"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if auth != "Bearer re_test" {
		t.Errorf("Authorization = %q", auth)
	}
	if got.From != "Super Sparrow <sparrow@example.com>" || len(got.To) != 1 || got.To[0] != "max@example.com" ||
		got.Subject != "Hello" || got.HTML != "<p>Body</p>" || got.Text != "Body" {
		t.Errorf("request = %+v", got)
	}
}

func TestResendReportsRefusals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"invalid from address"}`))
	}))
	defer server.Close()

	sender := NewResend("re_secret", "sparrow@example.com")
	sender.endpoint = server.URL

	err := sender.Send(context.Background(), Message{To: "max@example.com", Subject: "Hello", Text: "Body"})
	if err == nil || !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "invalid from address") {
		t.Fatalf("error = %v, want the status and Resend's message", err)
	}
	if strings.Contains(err.Error(), "re_secret") {
		t.Fatalf("error contains the API key: %v", err)
	}
}
