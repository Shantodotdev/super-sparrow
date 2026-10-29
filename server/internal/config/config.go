// Package config loads the server's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"strings"
)

// Config holds every setting the server needs. Each one is required; there are no defaults.
type Config struct {
	ListenAddr   string
	DatabasePath string
	PublicURL    *url.URL
	LogLevel     slog.Level
	// ResendAPIKey authenticates the server to Resend, which sends sign-in codes and invitations.
	ResendAPIKey string
	// EmailFrom is the sender of every email, such as "Super Sparrow <sparrow@example.com>".
	EmailFrom string
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header names the client's
	// address. It is empty when the server is reached directly.
	TrustedProxies []netip.Prefix
}

// Load reads the settings through getenv, which is os.Getenv outside tests. It returns one error
// that names every setting that is missing or malformed, so a bad .env is fixed in one pass.
func Load(getenv func(string) string) (Config, error) {
	var cfg Config
	var problems []error

	listenAddr, err := required(getenv, "SPARROW_LISTEN_ADDR")
	if err == nil {
		_, _, err = net.SplitHostPort(listenAddr)
		if err != nil {
			err = fmt.Errorf("SPARROW_LISTEN_ADDR must be host:port: %w", err)
		}
	}
	problems = append(problems, err)
	cfg.ListenAddr = listenAddr

	databasePath, err := required(getenv, "SPARROW_DATABASE_PATH")
	problems = append(problems, err)
	cfg.DatabasePath = databasePath

	publicURL, err := required(getenv, "SPARROW_PUBLIC_URL")
	if err == nil {
		cfg.PublicURL, err = parsePublicURL(publicURL)
	}
	problems = append(problems, err)

	logLevel, err := required(getenv, "SPARROW_LOG_LEVEL")
	if err == nil {
		err = cfg.LogLevel.UnmarshalText([]byte(logLevel))
		if err != nil {
			err = fmt.Errorf("SPARROW_LOG_LEVEL must be debug, info, warn, or error: %w", err)
		}
	}
	problems = append(problems, err)

	resendAPIKey, err := required(getenv, "SPARROW_RESEND_API_KEY")
	if err == nil && !strings.HasPrefix(resendAPIKey, "re_") {
		err = errors.New("SPARROW_RESEND_API_KEY must be a Resend API key, which starts with re_")
	}
	problems = append(problems, err)
	cfg.ResendAPIKey = resendAPIKey

	emailFrom, err := required(getenv, "SPARROW_EMAIL_FROM")
	if err == nil {
		_, err = mail.ParseAddress(emailFrom)
		if err != nil {
			err = fmt.Errorf("SPARROW_EMAIL_FROM must be an address such as Super Sparrow <sparrow@example.com>: %w", err)
		}
	}
	problems = append(problems, err)
	cfg.EmailFrom = emailFrom

	trustedProxies, err := required(getenv, "SPARROW_TRUSTED_PROXIES")
	if err == nil {
		cfg.TrustedProxies, err = parseTrustedProxies(trustedProxies)
	}
	problems = append(problems, err)

	if err := errors.Join(problems...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration:\n%w", err)
	}

	return cfg, nil
}

func required(getenv func(string) string, name string) (string, error) {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return value, nil
}

func parsePublicURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("SPARROW_PUBLIC_URL is not a URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("SPARROW_PUBLIC_URL must start with http:// or https://")
	}
	if parsed.Host == "" {
		return nil, errors.New("SPARROW_PUBLIC_URL must include a host")
	}

	return parsed, nil
}

// parseTrustedProxies reads "none" or a comma-separated list of IP addresses and CIDR ranges.
func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	if value == "none" {
		return nil, nil
	}

	var prefixes []netip.Prefix
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if prefix, err := netip.ParsePrefix(part); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("SPARROW_TRUSTED_PROXIES must be none or a comma-separated list of IP addresses and CIDR ranges, not %q", part)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return prefixes, nil
}
