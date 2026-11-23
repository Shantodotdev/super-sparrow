package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// DiscordLinkCode is a one-time code a person gives Super Sparrow in Discord to link their
// Discord account. Only its hash is stored.
type DiscordLinkCode struct {
	Code      string
	ExpiresAt time.Time
}

// StartDiscordLink makes a new Discord link code for the signed-in person, replacing any
// earlier one.
func (s *Service) StartDiscordLink(ctx context.Context, by store.Caller) (DiscordLinkCode, error) {
	code, err := newUserCode()
	if err != nil {
		return DiscordLinkCode{}, err
	}
	expires, err := s.store.CreateDiscordLinkCode(ctx, by, s.hashDiscordLinkCode(code))
	if err != nil {
		return DiscordLinkCode{}, err
	}
	return DiscordLinkCode{Code: formatUserCode(code), ExpiresAt: expires}, nil
}

// DiscordLinkCodeHash returns the hash a code given in Discord is stored under. It accepts the
// code in any case, with or without its dash.
func (s *Service) DiscordLinkCodeHash(code string) ([]byte, error) {
	normalized, ok := normalizedCode(code)
	if !ok {
		return nil, &store.ValidationError{Field: "code", Message: "must be the 8-letter link code from Sparrow Settings → Discord, such as BCDF-GHJK"}
	}
	return s.hashDiscordLinkCode(normalized), nil
}

func (s *Service) hashDiscordLinkCode(code string) []byte {
	mac := hmac.New(sha256.New, s.codeKey)
	mac.Write([]byte("discord-link\x00" + code))
	return mac.Sum(nil)
}
