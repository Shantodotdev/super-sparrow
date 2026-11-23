package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DiscordAnthropicModel is the Claude model used by the Discord mention assistant.
const DiscordAnthropicModel = "claude-haiku-5-5"

// DiscordTimeZone is the IANA time zone used for Discord daily status reports.
const DiscordTimeZone = "Asia/Dhaka"

var discordSettings = []string{"DISCORD_APPLICATION_ID", "DISCORD_GUILD_IDS", "DISCORD_GUILD_ID", "DISCORD_PUBLIC_KEY", "DISCORD_LISTEN_ADDR", "DISCORD_BOT_TOKEN"}

// Discord holds configuration for the approved Discord servers and Anthropic's Messages API.
type Discord struct {
	ApplicationID string
	GuildIDs      []string
	PublicKey     ed25519.PublicKey
	AnthropicKey  string
	Model         string
	ListenAddr    string
	BotToken      string
	TimeZone      *time.Location
}

// AllowsGuild reports whether an event came from an explicitly approved Discord server.
func (cfg Discord) AllowsGuild(id string) bool {
	return slices.Contains(cfg.GuildIDs, id)
}

// LoadDiscord leaves the integration off when no Discord settings are supplied.
func LoadDiscord(getenv func(string) string) (*Discord, error) {
	if !slices.ContainsFunc(discordSettings, func(name string) bool { return strings.TrimSpace(getenv(name)) != "" }) {
		return nil, nil
	}

	var cfg Discord
	var appErr, guildErr, keyErr error
	cfg.ApplicationID, appErr = discordID(getenv, "DISCORD_APPLICATION_ID")
	cfg.GuildIDs, guildErr = discordGuildIDs(getenv)
	cfg.Model = DiscordAnthropicModel
	token, tokenErr := required(getenv, "DISCORD_BOT_TOKEN")
	cfg.BotToken = token
	var zoneErr error
	cfg.TimeZone, zoneErr = time.LoadLocation(DiscordTimeZone)
	cfg.AnthropicKey, keyErr = required(getenv, "ANTHROPIC_API_KEY")
	if keyErr == nil && (!strings.HasPrefix(cfg.AnthropicKey, "sk-ant-") || strings.ContainsAny(cfg.AnthropicKey, " \r\n\t")) {
		keyErr = errors.New("ANTHROPIC_API_KEY must be an Anthropic API key")
	}

	var publicErr error
	cfg.PublicKey, publicErr = hex.DecodeString(strings.TrimSpace(getenv("DISCORD_PUBLIC_KEY")))
	if publicErr != nil || len(cfg.PublicKey) != ed25519.PublicKeySize {
		publicErr = errors.New("DISCORD_PUBLIC_KEY must contain 64 hexadecimal characters")
	}
	var listenErr error
	cfg.ListenAddr = strings.TrimSpace(getenv("DISCORD_LISTEN_ADDR"))
	if cfg.ListenAddr != "" {
		host, port, err := net.SplitHostPort(cfg.ListenAddr)
		address, addressErr := netip.ParseAddr(host)
		number, portErr := strconv.ParseUint(port, 10, 16)
		if err != nil || addressErr != nil || !address.IsLoopback() || portErr != nil || number == 0 {
			listenErr = errors.New("DISCORD_LISTEN_ADDR must be a loopback IP address and port")
		}
	}
	if err := errors.Join(appErr, guildErr, keyErr, publicErr, listenErr, tokenErr, zoneErr); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// discordGuildIDs prefers the allowlist whenever it is supplied. The legacy setting accepts
// one server ID so existing deployments can continue to use their current configuration.
func discordGuildIDs(getenv func(string) string) ([]string, error) {
	list := strings.TrimSpace(getenv("DISCORD_GUILD_IDS"))
	if list == "" {
		if strings.TrimSpace(getenv("DISCORD_GUILD_ID")) == "" {
			return nil, nil
		}
		id, err := discordID(getenv, "DISCORD_GUILD_ID")
		if err != nil {
			return nil, err
		}
		return []string{id}, nil
	}

	parts := strings.Split(list, ",")
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if !validDiscordID(id) {
			return nil, errors.New("DISCORD_GUILD_IDS must be a comma-separated list of Discord IDs")
		}
		if _, duplicate := seen[id]; !duplicate {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func discordID(getenv func(string) string, name string) (string, error) {
	value, err := required(getenv, name)
	if err != nil {
		return "", err
	}
	if !validDiscordID(value) {
		return "", errors.New(name + " must be a Discord ID")
	}
	return value, nil
}

func validDiscordID(value string) bool {
	id, err := strconv.ParseUint(value, 10, 64)
	return err == nil && id != 0 && strconv.FormatUint(id, 10) == value
}
