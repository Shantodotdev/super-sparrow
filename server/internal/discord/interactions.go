// Package discord receives Discord mentions and handles signed issue confirmations.
package discord

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/config"
	"github.com/shantodotdev/super-sparrow/server/internal/mcp"
	"github.com/shantodotdev/super-sparrow/server/internal/ratelimit"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// InteractionsPath is the endpoint Discord uses to deliver signed interactions.
const InteractionsPath = "/api/discord/interactions"

const discordAPI = "https://discord.com/api/v10"

const (
	maxPromptLength = 1500
	draftLifetime   = 10 * time.Minute
	confirmPrefix   = "sparrow:create:"
)

// Bot handles Discord mentions and issue confirmation buttons for approved guilds.
type Bot struct {
	cfg             config.Discord
	store           *store.Store
	tools           *mcp.Server
	model           modelClient
	client          *http.Client
	api             string
	logger          *slog.Logger
	now             func() time.Time
	perUser         *ratelimit.Limiter
	perPendingGuild *ratelimit.Limiter
	draftMu         sync.Mutex
	drafts          map[string]draft
}

type interaction struct {
	ID            string `json:"id"`
	Type          int    `json:"type"`
	ApplicationID string `json:"application_id"`
	GuildID       string `json:"guild_id"`
	Member        struct {
		User discordUser `json:"user"`
	} `json:"member"`
	Data struct {
		CustomID string `json:"custom_id"`
	} `json:"data"`
}

type response struct {
	Type int `json:"type"`
	Data any `json:"data,omitempty"`
}

type message struct {
	Content         string       `json:"content,omitempty"`
	Flags           int          `json:"flags,omitempty"`
	AllowedMentions allowed      `json:"allowed_mentions"`
	Components      *[]actionRow `json:"components,omitempty"`
}

type allowed struct {
	Parse []string `json:"parse"`
}

type actionRow struct {
	Type       int      `json:"type"`
	Components []button `json:"components"`
}

type button struct {
	Type     int    `json:"type"`
	Style    int    `json:"style"`
	Label    string `json:"label"`
	CustomID string `json:"custom_id"`
}

// New constructs a Discord bot with shared state for drafts and confirmations.
func New(cfg config.Discord, st *store.Store, tools *mcp.Server, logger *slog.Logger, now func() time.Time) *Bot {
	return &Bot{
		cfg: cfg, store: st, tools: tools, logger: logger, now: now,
		model:  anthropicClient{key: cfg.AnthropicKey, model: cfg.Model, client: newHTTPClient(20 * time.Second), endpoint: anthropicMessages},
		client: newHTTPClient(5 * time.Second), api: discordAPI,
		perUser: ratelimit.New(8, time.Minute, now), perPendingGuild: ratelimit.New(1, 5*time.Minute, now),
		drafts: make(map[string]draft),
	}
}

func (h *Bot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil {
		http.Error(w, "invalid Discord request body", http.StatusBadRequest)
		return
	}
	if !h.signed(r.Header, body) {
		http.Error(w, "invalid Discord signature", http.StatusUnauthorized)
		return
	}

	var in interaction
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "invalid Discord interaction", http.StatusBadRequest)
		return
	}
	if in.Type == 1 {
		writeResponse(w, response{Type: 1})
		return
	}
	source := h.guildSource(in.GuildID, in.Member.User.ID)
	if in.ApplicationID != h.cfg.ApplicationID || in.GuildID == "" || source.UserID == "" || in.Member.User.Bot || in.ID == "" {
		http.Error(w, "interaction is outside the approved Discord servers", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2300*time.Millisecond)
	defer cancel()
	approval, err := h.guildAccess(ctx, source)
	if err != nil {
		h.logger.WarnContext(ctx, "check Discord interaction approval", "error", err)
		http.Error(w, "Discord server approval unavailable", http.StatusServiceUnavailable)
		return
	}
	if approval != store.DiscordGuildApproved {
		http.Error(w, "interaction is outside the approved Discord servers", http.StatusForbidden)
		return
	}
	if in.Type != 3 {
		http.Error(w, "unsupported Discord interaction", http.StatusBadRequest)
		return
	}
	writeResponse(w, h.confirm(ctx, source, in.Data.CustomID))
}

// signed checks the signature before any interaction is used. A short timestamp window limits replay.
func (h *Bot) signed(headers http.Header, body []byte) bool {
	timestamp := headers.Get("X-Signature-Timestamp")
	signature, err := hex.DecodeString(headers.Get("X-Signature-Ed25519"))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	age := h.now().Sub(time.Unix(seconds, 0))
	if age > 5*time.Minute || age < -time.Minute {
		return false
	}
	return ed25519.Verify(h.cfg.PublicKey, append([]byte(timestamp), body...), signature)
}

func (h *Bot) userError(ctx context.Context, operation string, err error) string {
	var validation *store.ValidationError
	switch {
	case errors.As(err, &validation), errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrForbidden), errors.Is(err, store.ErrConflict):
		return store.Explain(err)
	default:
		h.logger.WarnContext(ctx, operation, "error", err)
		return "Sparrow couldn’t complete this request. Try again in a moment."
	}
}

func plain(content string) message {
	return message{Content: shorten(content, 1850), AllowedMentions: allowed{Parse: []string{}}}
}

func private(content string) response {
	data := plain(content)
	data.Flags = 64
	return response{Type: 4, Data: data}
}

func writeResponse(w http.ResponseWriter, reply response) {
	body, err := json.Marshal(reply)
	if err != nil {
		http.Error(w, "cannot encode Discord response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

func shorten(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}
