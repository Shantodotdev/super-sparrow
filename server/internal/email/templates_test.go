package email

import (
	"strings"
	"testing"
)

func TestSignInCodeEmail(t *testing.T) {
	message, err := SignInCode("max@example.com", "042917", 10)
	if err != nil {
		t.Fatalf("SignInCode: %v", err)
	}

	if message.To != "max@example.com" || message.Subject != "042917 is your Super Sparrow sign-in code" {
		t.Errorf("message = %+v", message)
	}
	for _, body := range []string{message.HTML, message.Text} {
		if !strings.Contains(body, "042917") || !strings.Contains(body, "10 minutes") {
			t.Errorf("body lacks the code or its expiry: %q", body)
		}
	}
}

func TestInvitationEmailEscapesNames(t *testing.T) {
	message, err := Invitation("max@example.com", `Olive <script>alert(1)</script>`, "https://sparrow.example.com")
	if err != nil {
		t.Fatalf("Invitation: %v", err)
	}

	if strings.Contains(message.HTML, "<script>") {
		t.Error("inviter name was not escaped")
	}
	if !strings.Contains(message.HTML, `href="https://sparrow.example.com"`) || !strings.Contains(message.Text, "https://sparrow.example.com") {
		t.Error("invitation lacks the sign-in address")
	}
}
