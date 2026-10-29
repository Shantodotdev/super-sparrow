package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/email"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// fakeMailer records sent messages, and fails every send while fail is set.
type fakeMailer struct {
	mu   sync.Mutex
	sent []email.Message
	fail bool
}

func (m *fakeMailer) Send(_ context.Context, message email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return errors.New("mail server unavailable")
	}
	m.sent = append(m.sent, message)
	return nil
}

func (m *fakeMailer) messages() []email.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]email.Message(nil), m.sent...)
}

var codePattern = regexp.MustCompile(`code is (\d{6})`)

type fixture struct {
	ctx     context.Context
	store   *store.Store
	service *Service
	mailer  *fakeMailer
	owner   store.Actor
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	st := store.New(db, time.Now)
	owner, err := st.CreateFirstOwner(ctx, "Olive Owner", "olive@example.com")
	if err != nil {
		t.Fatalf("CreateFirstOwner: %v", err)
	}

	mailer := &fakeMailer{}
	publicURL, _ := url.Parse("https://sparrow.example.com")
	service := New(st, mailer, publicURL, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)

	return &fixture{ctx: ctx, store: st, service: service, mailer: mailer, owner: owner}
}

// codeFor requests a code for address and returns the one emailed.
func (f *fixture) codeFor(t *testing.T, address string) string {
	t.Helper()
	if err := f.service.RequestCode(f.ctx, address, "203.0.113.7"); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	f.service.Wait()

	sent := f.mailer.messages()
	if len(sent) == 0 {
		t.Fatal("no email sent")
	}
	match := codePattern.FindStringSubmatch(sent[len(sent)-1].Text)
	if match == nil {
		t.Fatalf("no code in email: %q", sent[len(sent)-1].Text)
	}
	return match[1]
}

func TestSignInWithEmailedCode(t *testing.T) {
	f := newFixture(t)
	code := f.codeFor(t, "Olive@Example.com")

	if sent := f.mailer.messages(); sent[0].To != "olive@example.com" || !strings.Contains(sent[0].Subject, code) {
		t.Errorf("email = %+v", sent[0])
	}

	signIn, err := f.service.VerifyCode(f.ctx, "olive@example.com", code, "Firefox on Linux")
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}
	if signIn.Actor.ID != f.owner.ID || len(signIn.Secret) != secretLength {
		t.Fatalf("sign-in = %+v", signIn)
	}

	checked, err := f.service.AuthenticateSession(f.ctx, signIn.Secret)
	if err != nil || checked.Actor.ID != f.owner.ID {
		t.Fatalf("AuthenticateSession = %+v, %v", checked, err)
	}
}

func TestStrangersGetNoEmailButTheSameAnswer(t *testing.T) {
	f := newFixture(t)

	if err := f.service.RequestCode(f.ctx, "stranger@example.com", "203.0.113.7"); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	f.service.Wait()

	if sent := f.mailer.messages(); len(sent) != 0 {
		t.Fatalf("sent %d emails to a stranger", len(sent))
	}
}

func TestVerifyRejectsMalformedAndWrongCodes(t *testing.T) {
	f := newFixture(t)
	code := f.codeFor(t, "olive@example.com")

	var validation *store.ValidationError
	if _, err := f.service.VerifyCode(f.ctx, "olive@example.com", "12ab56", "Firefox on Linux"); !errors.As(err, &validation) || validation.Field != "code" {
		t.Errorf("malformed code error = %v", err)
	}

	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	if _, err := f.service.VerifyCode(f.ctx, "olive@example.com", wrong, "Firefox on Linux"); !errors.Is(err, store.ErrInvalidCode) {
		t.Errorf("wrong code error = %v", err)
	}
}

func TestCodeRequestsAreRateLimitedPerEmailAndPerIP(t *testing.T) {
	f := newFixture(t)

	for i := range codeRequestsPerEmail {
		if err := f.service.RequestCode(f.ctx, "olive@example.com", "203.0.113.7"); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	var limited *RateLimitError
	if err := f.service.RequestCode(f.ctx, "olive@example.com", "198.51.100.1"); !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("over the per-email limit: %v", err)
	}

	for i := codeRequestsPerEmail; i < codeRequestsPerIP; i++ {
		if err := f.service.RequestCode(f.ctx, "someone"+string(rune('a'+i))+"@example.com", "203.0.113.7"); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	if err := f.service.RequestCode(f.ctx, "fresh@example.com", "203.0.113.7"); !errors.As(err, &limited) {
		t.Fatalf("over the per-IP limit: %v", err)
	}
	f.service.Wait()
}

func TestTokensAuthenticateAsTheAgent(t *testing.T) {
	f := newFixture(t)

	created, err := f.service.CreateToken(f.ctx, store.Caller{ActorID: f.owner.ID}, "claude-code", "Olive · Claude Code · laptop")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if !strings.HasPrefix(created.Secret, TokenPrefix) || !strings.HasSuffix(created.Secret, strings.TrimPrefix(created.Token.DisplayHint, TokenPrefix+"…")) {
		t.Fatalf("secret %q does not match hint %q", created.Secret, created.Token.DisplayHint)
	}

	checked, err := f.service.AuthenticateToken(f.ctx, created.Secret)
	if err != nil || checked.Actor.ID != created.Token.AgentActorID || checked.Actor.DisplayName != "Claude Code (Olive Owner)" {
		t.Fatalf("AuthenticateToken = %+v, %v", checked.Actor, err)
	}

	for _, bad := range []string{"", "ssp_short", strings.TrimPrefix(created.Secret, TokenPrefix), created.Secret[:len(created.Secret)-1] + "x"} {
		if _, err := f.service.AuthenticateToken(f.ctx, bad); !errors.Is(err, store.ErrUnauthenticated) {
			t.Errorf("token %q: error = %v", bad, err)
		}
	}
}

func TestInviteSendsAnEmail(t *testing.T) {
	f := newFixture(t)
	by := store.Caller{ActorID: f.owner.ID}

	actor, err := f.service.Invite(f.ctx, by, store.NewHuman{DisplayName: "Max Member", Email: "max@example.com", Role: store.RoleMember})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	sent := f.mailer.messages()
	if len(sent) != 1 || sent[0].To != "max@example.com" || !strings.Contains(sent[0].Text, "https://sparrow.example.com") ||
		!strings.Contains(sent[0].Subject, "Olive Owner") {
		t.Fatalf("invitation = %+v", sent)
	}

	f.mailer.fail = true
	second, err := f.service.Invite(f.ctx, by, store.NewHuman{DisplayName: "Nia", Email: "nia@example.com", Role: store.RoleMember})
	if !errors.Is(err, ErrInvitationNotSent) || second.ID == "" || second.ID == actor.ID {
		t.Fatalf("failed invitation = %+v, %v", second, err)
	}
}
