package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templateFiles embed.FS

// Each email is the shared layout with its own content block, parsed once at startup.
var (
	signInCodeTemplate = template.Must(template.ParseFS(templateFiles, "templates/layout.html", "templates/sign_in_code.html"))
	invitationTemplate = template.Must(template.ParseFS(templateFiles, "templates/layout.html", "templates/invitation.html"))
)

// SignInCode returns the email that carries a sign-in code, valid for minutes.
func SignInCode(to, code string, minutes int) (Message, error) {
	subject := code + " is your Super Sparrow sign-in code"
	footer := "If you didn't try to sign in, you can ignore this email."

	html, err := render(signInCodeTemplate, map[string]any{"Subject": subject, "Footer": footer, "Code": code, "Minutes": minutes})
	if err != nil {
		return Message{}, err
	}

	return Message{
		To:      to,
		Subject: subject,
		HTML:    html,
		Text:    fmt.Sprintf("Your Super Sparrow sign-in code is %s.\n\nIt expires in %d minutes.\n\n%s\n", code, minutes, footer),
	}, nil
}

// Invitation returns the email that tells a new teammate they were added, with the address to
// open and the email to sign in with.
func Invitation(to, inviter, url string) (Message, error) {
	subject := inviter + " added you to Super Sparrow"
	footer := "Super Sparrow has no passwords. Each sign-in uses a code sent to this address."

	html, err := render(invitationTemplate, map[string]any{"Subject": subject, "Footer": footer, "Inviter": inviter, "Email": to, "URL": url})
	if err != nil {
		return Message{}, err
	}

	return Message{
		To:      to,
		Subject: subject,
		HTML:    html,
		Text:    fmt.Sprintf("%s added you to the team on Super Sparrow.\n\nOpen %s and sign in with %s. We'll email you a code.\n\n%s\n", inviter, url, to, footer),
	}, nil
}

func render(tmpl *template.Template, data map[string]any) (string, error) {
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "layout", data); err != nil {
		return "", fmt.Errorf("render email: %w", err)
	}
	return out.String(), nil
}
