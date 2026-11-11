package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var signInCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

// login signs the CLI in and stores its session token. By default the person approves the
// sign-in in the desktop app; --email signs in with an emailed code instead.
func (r *runner) login(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	rawURL := flags.String("url", "", "")
	emailAddress := flags.String("email", "", "")
	noOpen := flags.Bool("no-open", false, "")
	if _, err := parse(flags, args, 0); err != nil {
		return err
	}

	baseURL, err := r.env.serverURL(*rawURL)
	if err != nil {
		return err
	}
	// Show the whole address before anything is sent, so a hostile SPARROW_URL is visible.
	fmt.Fprintf(r.errOut, "Signing in to %s\n", baseURL)

	var verified signedIn
	if strings.TrimSpace(*emailAddress) != "" {
		verified, err = r.loginWithEmail(ctx, baseURL, strings.TrimSpace(*emailAddress))
	} else {
		verified, err = r.loginInApp(ctx, baseURL, !*noOpen)
	}
	if err != nil {
		return err
	}
	return r.storeLogin(ctx, baseURL, verified)
}

// signedIn is the server's answer to a finished sign-in: the CLI's session token and who it is.
type signedIn struct {
	Token string `json:"token"`
	Actor struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	} `json:"actor"`
}

// loginInApp asks the server for a sign-in request, opens the desktop app's approval screen, and
// waits until the person approves or denies it there, or it expires.
func (r *runner) loginInApp(ctx context.Context, baseURL *url.URL, open bool) (signedIn, error) {
	anonymous := newClient(baseURL, "", "", "")
	data, err := anonymous.do(ctx, http.MethodPost, "/api/auth/cli/login", nil, map[string]string{"device_label": r.deviceLabel()})
	if err != nil {
		return signedIn{}, fmt.Errorf("start the sign-in: %w", err)
	}
	var started struct {
		DeviceCode string    `json:"device_code"`
		UserCode   string    `json:"user_code"`
		ExpiresAt  time.Time `json:"expires_at"`
		Interval   int       `json:"interval_seconds"`
		ApproveURL string    `json:"approve_url"`
	}
	if err := json.Unmarshal(data, &started); err != nil || started.DeviceCode == "" || started.Interval < 1 {
		return signedIn{}, errors.New("the server did not start a sign-in; is it a Super Sparrow server?")
	}

	fmt.Fprintf(r.errOut, "Approve this sign-in in the Super Sparrow app. Code: %s\n", started.UserCode)
	if open && r.env.OpenURL != nil {
		if err := r.env.OpenURL(started.ApproveURL); err != nil {
			fmt.Fprintf(r.errOut, "Could not open the app (%v).\n", err)
		}
	}
	fmt.Fprintln(r.errOut, "If the app did not open, open it and choose Settings → Connect CLI, then enter the code.")
	fmt.Fprintln(r.errOut, "Waiting for approval…")

	// Wait at least the interval between checks, and stop when the request expires.
	ticker := time.NewTicker(time.Duration(started.Interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return signedIn{}, ctx.Err()
		case <-ticker.C:
		}

		data, err := anonymous.do(ctx, http.MethodPost, "/api/auth/cli/login/poll", nil, map[string]string{"device_code": started.DeviceCode})
		if err != nil {
			return signedIn{}, fmt.Errorf("check the sign-in: %w", err)
		}
		var polled struct {
			Status string `json:"status"`
			signedIn
		}
		if err := json.Unmarshal(data, &polled); err != nil {
			return signedIn{}, fmt.Errorf("read the sign-in status: %w", err)
		}

		switch polled.Status {
		case "pending":
			continue
		case "claimed":
			if polled.Token == "" {
				return signedIn{}, errors.New("the server did not return a session token")
			}
			return polled.signedIn, nil
		case "denied":
			return signedIn{}, errors.New("the sign-in was denied in the app")
		case "expired":
			return signedIn{}, errors.New("the sign-in expired before it was approved; run sparrow login again")
		default:
			return signedIn{}, fmt.Errorf("the server reported an unknown sign-in status %q", polled.Status)
		}
	}
}

// loginWithEmail signs in with a code emailed to address.
func (r *runner) loginWithEmail(ctx context.Context, baseURL *url.URL, address string) (signedIn, error) {
	// Ask the server to email a code, then trade the code for a session token.
	anonymous := newClient(baseURL, "", "", "")
	if _, err := anonymous.do(ctx, http.MethodPost, "/api/auth/code", nil, map[string]string{"email": address}); err != nil {
		return signedIn{}, fmt.Errorf("request a sign-in code: %w", err)
	}
	fmt.Fprintf(r.errOut, "If %s can sign in, a 6-digit code was sent to it.\n", address)

	data, err := r.verifyCode(ctx, anonymous, bufio.NewReader(r.stdin), address, r.deviceLabel())
	if err != nil {
		return signedIn{}, err
	}

	var verified signedIn
	if err := json.Unmarshal(data, &verified); err != nil || verified.Token == "" {
		return signedIn{}, errors.New("the server did not return a session token")
	}
	return verified, nil
}

// storeLogin keeps a new session in the keychain and ends the session it replaces.
func (r *runner) storeLogin(ctx context.Context, baseURL *url.URL, verified signedIn) error {
	// Remember the login this one replaces, so its session can be ended once the new one is stored.
	previous, previousErr := r.env.store().load()

	warning, err := r.env.store().save(credentials{URL: baseURL.String(), Token: verified.Token})
	if err != nil {
		return fmt.Errorf("store the login: %w", err)
	}
	if warning != "" {
		fmt.Fprintln(r.errOut, warning)
	}
	if previousErr == nil && previous.Token != verified.Token {
		r.revokeSession(ctx, previous)
	}
	fmt.Fprintf(r.out, "Signed in to %s as %s (%s).\n", baseURL, verified.Actor.DisplayName, verified.Actor.Email)
	return nil
}

// deviceLabel names this computer in the person's session list.
func (r *runner) deviceLabel() string {
	hostname, err := r.env.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown host"
	}
	return truncateRunes(hostname+" · CLI", maxDeviceLabelLength)
}

// maxDeviceLabelLength is the longest device label the CLI sends.
const maxDeviceLabelLength = 100

// maxCodeAttempts is how many times login asks for the emailed code before giving up. The server
// cancels the code after five wrong guesses, so this stays below that.
const maxCodeAttempts = 3

// verifyCode asks for the emailed code and trades it for a session. A wrong code is asked for
// again, up to maxCodeAttempts times, without requesting another email. It returns the server's
// response body.
func (r *runner) verifyCode(ctx context.Context, anonymous *client, input *bufio.Reader, address, label string) (json.RawMessage, error) {
	for attempt := 1; ; attempt++ {
		code, err := r.prompt(input, "Code: ")
		if err != nil {
			return nil, err
		}
		code = strings.ReplaceAll(code, " ", "")

		var failure error
		if !signInCodePattern.MatchString(code) {
			failure = errors.New("the code must be 6 digits")
		} else {
			data, err := anonymous.do(ctx, http.MethodPost, "/api/auth/cli/verify", nil, map[string]string{
				"email": address, "code": code, "device_label": label,
			})
			if err == nil {
				return data, nil
			}

			var apiFailure *apiError
			if !errors.As(err, &apiFailure) || apiFailure.Status != http.StatusBadRequest || apiFailure.Field != "code" {
				return nil, fmt.Errorf("verify the sign-in code: %w", err)
			}
			failure = fmt.Errorf("verify the sign-in code: %w", err)
		}

		if attempt == maxCodeAttempts {
			return nil, failure
		}
		fmt.Fprintf(r.errOut, "%v; try again (%d of %d).\n", failure, attempt, maxCodeAttempts)
	}
}

// revokeSession ends a stored session on its server. It is best effort: the person is already
// signed in again, so a failure is only mentioned.
func (r *runner) revokeSession(ctx context.Context, creds credentials) {
	baseURL, err := parseServerURL(creds.URL)
	if err != nil {
		return
	}
	if _, err := newClient(baseURL, creds.Token, "", "").do(ctx, http.MethodPost, "/api/auth/sign-out", nil, nil); err != nil {
		var failure *apiError
		if !errors.As(err, &failure) || failure.Status != http.StatusUnauthorized {
			fmt.Fprintf(r.errOut, "warning: could not end the previous session on %s: %v\n", baseURL, err)
		}
	}
}

// truncateRunes returns text cut to at most limit characters.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

// prompt writes label to the error stream and reads one line from input.
func (r *runner) prompt(input *bufio.Reader, label string) (string, error) {
	fmt.Fprint(r.errOut, label)
	line, err := input.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read %s from standard input: %w", strings.ToLower(strings.TrimSuffix(strings.TrimSpace(label), ":")), err)
	}
	return strings.TrimSpace(line), nil
}

// logout revokes the stored CLI session and deletes the stored login.
func (r *runner) logout(ctx context.Context, args []string) error {
	if _, err := parse(flag.NewFlagSet("logout", flag.ContinueOnError), args, 0); err != nil {
		return err
	}

	store := r.env.store()
	creds, err := store.load()
	if errors.Is(err, errNotSignedIn) {
		fmt.Fprintln(r.out, "Not signed in.")
		return nil
	}
	if err != nil {
		// A damaged login cannot be revoked; removing it is still what the person asked for.
		if deleteErr := store.delete(); deleteErr != nil {
			return errors.Join(err, deleteErr)
		}
		var keychainErr *keychainReadError
		if errors.As(err, &keychainErr) {
			fmt.Fprintf(r.out, "Could not read the keychain (%v); removed any stored login file, but could not revoke a session.\n", keychainErr.err)
			return nil
		}
		fmt.Fprintln(r.out, "Removed the damaged stored login.")
		return nil
	}

	// The sign-out must end the person's session, so it never carries an agent header.
	var revokeErr error
	if baseURL, err := parseServerURL(creds.URL); err != nil {
		revokeErr = err
	} else if _, err := newClient(baseURL, creds.Token, "", "").do(ctx, http.MethodPost, "/api/auth/sign-out", nil, nil); err != nil {
		var failure *apiError
		if !errors.As(err, &failure) || failure.Status != http.StatusUnauthorized {
			revokeErr = err
		}
	}

	if err := store.delete(); err != nil {
		return errors.Join(revokeErr, err)
	}
	if revokeErr != nil {
		return fmt.Errorf("removed the stored login, but could not revoke the session on the server: %w", revokeErr)
	}
	fmt.Fprintln(r.out, "Signed out.")
	return nil
}

// whoami shows the signed-in actor, the server, how the CLI is authorized, and which agent it
// acts as. With --json the actor response gains server, auth (login or token), and agent fields.
func (r *runner) whoami(ctx context.Context, args []string) (json.RawMessage, error) {
	if _, err := parse(flag.NewFlagSet("whoami", flag.ContinueOnError), args, 0); err != nil {
		return nil, err
	}

	data, err := r.client.do(ctx, http.MethodGet, "/api/me", nil, nil)
	if err != nil {
		return nil, err
	}
	if r.asJSON {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, fmt.Errorf("decode the actor: %w", err)
		}

		var agent any
		if r.conn.agent != "" {
			agent = r.conn.agent
		}
		for name, value := range map[string]any{"server": r.client.baseURL.String(), "auth": string(r.conn.kind), "agent": agent} {
			if fields[name], err = json.Marshal(value); err != nil {
				return nil, fmt.Errorf("encode %s: %w", name, err)
			}
		}
		return json.Marshal(fields)
	}

	text, err := renderText(data)
	if err != nil {
		return nil, err
	}
	auth := "stored login"
	if r.conn.kind == authToken {
		auth = "SPARROW_TOKEN (agent token)"
	}
	agent := "none (acting as the person)"
	switch {
	case r.conn.agent != "":
		agent = r.conn.agent + " (" + r.conn.agentSource + ")"
	case r.conn.kind == authToken:
		agent = "the agent token's agent"
	}
	fmt.Fprintf(r.out, "%s\nserver: %s\nauth: %s\nagent: %s\n", text, r.client.baseURL, auth, agent)
	return nil, nil
}
