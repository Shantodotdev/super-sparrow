package httpserver

import (
	"net/http"
	"strings"
	"testing"
)

type startedLogin struct {
	DeviceCode string `json:"device_code"`
	UserCode   string `json:"user_code"`
	Interval   int    `json:"interval_seconds"`
	ApproveURL string `json:"approve_url"`
}

func (f *fixture) startCLILogin() startedLogin {
	f.t.Helper()
	response := f.request(http.MethodPost, "/api/auth/cli/login", "", map[string]string{"device_label": "dev box · CLI"})
	wantStatus(f.t, response, http.StatusOK)

	var started startedLogin
	decode(f.t, response, &started)
	return started
}

// pollCLILogin polls once and returns the status and, once claimed, the token.
func (f *fixture) pollCLILogin(deviceCode string) (string, string) {
	f.t.Helper()
	response := f.request(http.MethodPost, "/api/auth/cli/login/poll", "", map[string]string{"device_code": deviceCode})
	wantStatus(f.t, response, http.StatusOK)

	var body struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}
	decode(f.t, response, &body)
	return body.Status, body.Token
}

func TestCLILoginApprovedInTheAppSignsTheCLIIn(t *testing.T) {
	f := newFixture(t)
	session := f.signIn("olive@example.com")

	started := f.startCLILogin()
	if !strings.HasPrefix(started.DeviceCode, "ssd_") || len(started.UserCode) != 9 || started.UserCode[4] != '-' || started.Interval != 2 {
		t.Fatalf("started %+v", started)
	}
	if started.ApproveURL != "super-sparrow://cli-login?code="+started.UserCode {
		t.Fatalf("approve URL %q", started.ApproveURL)
	}
	if status, token := f.pollCLILogin(started.DeviceCode); status != "pending" || token != "" {
		t.Fatalf("before approval: %q %q", status, token)
	}

	// The approval screen shows the request; a lowercase code without its dash also works.
	typed := strings.ToLower(strings.ReplaceAll(started.UserCode, "-", ""))
	response := f.request(http.MethodGet, "/api/auth/cli/requests/"+typed, session, nil)
	wantStatus(t, response, http.StatusOK)
	var shown cliLoginView
	decode(t, response, &shown)
	if shown.DeviceLabel != "dev box · CLI" || !shown.SameAddress || shown.RequestedFrom == "" {
		t.Fatalf("shown %+v", shown)
	}

	wantStatus(t, f.request(http.MethodPost, "/api/auth/cli/requests/"+started.UserCode+"/approve", session, nil), http.StatusOK)

	status, token := f.pollCLILogin(started.DeviceCode)
	if status != "claimed" || !strings.HasPrefix(token, "ssc_") {
		t.Fatalf("after approval: %q %q", status, token)
	}
	if me := f.meOf(token, nil); me.Email != "olive@example.com" {
		t.Fatalf("CLI acts as %+v", me)
	}

	// The token is handed out once.
	if status, token := f.pollCLILogin(started.DeviceCode); status != "expired" || token != "" {
		t.Fatalf("second claim: %q %q", status, token)
	}
}

func TestCLILoginDeniedNeverSignsIn(t *testing.T) {
	f := newFixture(t)
	session := f.signIn("olive@example.com")
	started := f.startCLILogin()

	wantStatus(t, f.request(http.MethodPost, "/api/auth/cli/requests/"+started.UserCode+"/deny", session, nil), http.StatusOK)

	if status, token := f.pollCLILogin(started.DeviceCode); status != "denied" || token != "" {
		t.Fatalf("after denial: %q %q", status, token)
	}
	// A decided request cannot be approved afterwards.
	wantStatus(t, f.request(http.MethodPost, "/api/auth/cli/requests/"+started.UserCode+"/approve", session, nil), http.StatusNotFound)
}

func TestCLISessionsAndAgentsCannotApproveCLILogins(t *testing.T) {
	f := newFixture(t)
	session := f.signIn("olive@example.com")
	cli := f.cliSignIn("olive@example.com")
	agentToken, _ := f.createToken(session)
	started := f.startCLILogin()
	path := "/api/auth/cli/requests/" + started.UserCode + "/approve"

	wantStatus(t, f.request(http.MethodPost, path, cli, nil), http.StatusForbidden)
	wantStatus(t, f.request(http.MethodPost, path, agentToken, nil), http.StatusForbidden)
	wantStatus(t, f.requestWith(http.MethodPost, path, cli, map[string]string{agentHeader: "claude-code"}, nil), http.StatusForbidden)
	wantStatus(t, f.request(http.MethodPost, path, "", nil), http.StatusUnauthorized)

	if status, _ := f.pollCLILogin(started.DeviceCode); status != "pending" {
		t.Fatalf("status %q after refused approvals", status)
	}
}

func TestCLILoginRejectsBadCodes(t *testing.T) {
	f := newFixture(t)
	session := f.signIn("olive@example.com")

	wantField(t, f.request(http.MethodGet, "/api/auth/cli/requests/AEIO-0000", session, nil), "user_code")
	wantStatus(t, f.request(http.MethodGet, "/api/auth/cli/requests/BBBB-BBBB", session, nil), http.StatusNotFound)
	if status, _ := f.pollCLILogin("ssd_not-a-real-code"); status != "expired" {
		t.Fatalf("unknown device code status %q", status)
	}
}

func TestCLILoginRequiresADeviceLabel(t *testing.T) {
	f := newFixture(t)
	wantField(t, f.request(http.MethodPost, "/api/auth/cli/login", "", map[string]string{"device_label": " "}), "device_label")
}
