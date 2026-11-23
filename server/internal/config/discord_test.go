package config

import (
	"slices"
	"strings"
	"testing"
)

func TestLoadDiscordConfiguration(t *testing.T) {
	values := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-test"}
	getenv := func(key string) string { return values[key] }
	if cfg, err := LoadDiscord(getenv); cfg != nil || err != nil {
		t.Fatalf("an Anthropic key alone enabled Discord: %v", err)
	}
	values["DISCORD_APPLICATION_ID"] = "123"
	if _, err := LoadDiscord(getenv); err == nil || !strings.Contains(err.Error(), "DISCORD_PUBLIC_KEY") {
		t.Fatalf("incomplete Discord settings were accepted: %v", err)
	}
	values["DISCORD_GUILD_ID"] = "456"
	values["DISCORD_PUBLIC_KEY"] = strings.Repeat("12", 32)
	values["DISCORD_BOT_TOKEN"] = "test-token"
	values["DISCORD_LISTEN_ADDR"] = "0.0.0.0:8080"
	if _, err := LoadDiscord(getenv); err == nil || !strings.Contains(err.Error(), "DISCORD_LISTEN_ADDR") {
		t.Fatalf("a wildcard listener was accepted: %v", err)
	}
	values["DISCORD_LISTEN_ADDR"] = "127.0.0.1:8080"
	cfg, err := LoadDiscord(getenv)
	if err != nil || cfg.Model != DiscordAnthropicModel || cfg.TimeZone.String() != DiscordTimeZone || cfg.ListenAddr != values["DISCORD_LISTEN_ADDR"] {
		t.Fatalf("valid Discord settings were rejected: %v", err)
	}
	if !cfg.AllowsGuild("456") || cfg.AllowsGuild("789") {
		t.Fatal("legacy setting did not approve exactly one guild")
	}

	delete(values, "DISCORD_GUILD_ID")
	cfg, err = LoadDiscord(getenv)
	if err != nil || len(cfg.GuildIDs) != 0 {
		t.Fatalf("Discord should be configurable with the approval UI alone: %v", err)
	}
	values["DISCORD_GUILD_ID"] = "456"

	values["DISCORD_GUILD_IDS"] = "789, 1234, 5678, 789"
	cfg, err = LoadDiscord(getenv)
	if err != nil {
		t.Fatalf("valid Discord guild allowlist was rejected: %v", err)
	}
	if !slices.Equal(cfg.GuildIDs, []string{"789", "1234", "5678"}) || cfg.AllowsGuild("456") {
		t.Fatalf("guild allowlist must override legacy setting and deduplicate IDs: %v", cfg.GuildIDs)
	}
	for _, id := range cfg.GuildIDs {
		if !cfg.AllowsGuild(id) {
			t.Fatalf("approved guild %s was rejected", id)
		}
	}
	for _, list := range []string{"789,", "789, ,1234", "789,nope", "789,0", "789,01234", "789,18446744073709551616"} {
		values["DISCORD_GUILD_IDS"] = list
		if _, err := LoadDiscord(getenv); err == nil || !strings.Contains(err.Error(), "DISCORD_GUILD_IDS") {
			t.Fatalf("malformed Discord guild allowlist %q was accepted: %v", list, err)
		}
	}
}
