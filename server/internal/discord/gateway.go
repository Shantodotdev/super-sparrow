package discord

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

type mention struct {
	source    store.DiscordIdentity
	guildName string
	channelID string
	messageID string
	userName  string
	prompt    string
}

// Run listens for mention messages until the server shuts down. DiscordGo maintains the
// heartbeat and resumes interrupted sessions. A bounded queue keeps inference off the socket.
func (h *Bot) Run(ctx context.Context) {
	session, err := discordgo.New("Bot " + h.cfg.BotToken)
	if err != nil {
		h.logger.ErrorContext(ctx, "configure Discord gateway", "error", err)
		return
	}
	session.Client = h.client
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages
	session.SyncEvents = true

	pending := make(chan mention, 16)
	session.AddHandler(func(_ *discordgo.Session, event *discordgo.Ready) {
		guilds, err := h.GuildApprovals(ctx)
		if err != nil {
			h.logger.WarnContext(ctx, "read Discord guild approvals for gateway", "error", err)
		}
		approved := make(map[string]struct{}, len(guilds))
		for _, guild := range guilds {
			if guild.Status == store.DiscordGuildApproved {
				approved[guild.ID] = struct{}{}
			}
		}
		connected := 0
		for _, guild := range event.Guilds {
			if guild != nil {
				if _, ok := approved[guild.ID]; ok {
					connected++
				}
			}
		}
		h.logger.InfoContext(ctx, "Discord gateway connected",
			"guild_available", connected > 0,
			"approved_guilds_connected", connected,
			"approved_guilds_configured", len(h.cfg.GuildIDs),
			"approved_guilds_total", len(approved))
	})
	session.AddHandler(func(_ *discordgo.Session, event *discordgo.GuildCreate) {
		if event == nil || event.Guild == nil || event.ID == "" || ctx.Err() != nil {
			return
		}
		h.observeGuild(ctx, event.ID, event.Name)
	})
	session.AddHandler(func(session *discordgo.Session, event *discordgo.MessageCreate) {
		request, ok := h.parseMention(event.Message)
		if !ok || ctx.Err() != nil {
			return
		}
		if guild, err := session.State.Guild(request.source.GuildID); err == nil {
			request.guildName = shorten(strings.Join(strings.Fields(guild.Name), " "), 100)
		}
		select {
		case pending <- request:
		default:
			h.logger.WarnContext(ctx, "Discord mention queue is full")
		}
	})

	for ctx.Err() == nil {
		if err := session.Open(); err == nil {
			break
		} else {
			h.logger.WarnContext(ctx, "connect Discord gateway", "error", err)
		}
		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
	defer session.Close()

	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for ctx.Err() == nil {
				select {
				case <-ctx.Done():
					return
				case request := <-pending:
					h.respondToMention(ctx, request)
				}
			}
		})
	}
	workers.Wait()
}

// parseMention authenticates the author and guild from the Gateway event. The mention field
// and the text must both reference this bot, so automatic reply pings don't trigger inference.
func (h *Bot) parseMention(event *discordgo.Message) (mention, bool) {
	if event == nil || event.GuildID == "" || event.Author == nil || event.Author.Bot ||
		event.Author.ID == "" || event.WebhookID != "" || event.ID == "" || event.ChannelID == "" {
		return mention{}, false
	}
	id := h.cfg.ApplicationID
	if !slices.ContainsFunc(event.Mentions, func(user *discordgo.User) bool { return user != nil && user.ID == id }) {
		return mention{}, false
	}
	tag, nicknameTag := "<@"+id+">", "<@!"+id+">"
	if !strings.Contains(event.Content, tag) && !strings.Contains(event.Content, nicknameTag) {
		return mention{}, false
	}
	prompt := strings.TrimSpace(strings.NewReplacer(tag, "", nicknameTag, "").Replace(event.Content))
	userName := event.Author.GlobalName
	if userName == "" {
		userName = event.Author.Username
	}
	return mention{
		source:    h.guildSource(event.GuildID, event.Author.ID),
		channelID: event.ChannelID, messageID: event.ID, userName: shorten(oneLine(userName), 100), prompt: prompt,
	}, true
}

// respondToMention checks approval before the model or workspace tools can see the request.
func (h *Bot) respondToMention(ctx context.Context, request mention) {
	status, err := h.guildAccess(ctx, request.source)
	if err != nil {
		h.logger.WarnContext(ctx, "check Discord mention approval", "error", err)
		return
	}
	if status == store.DiscordGuildRevoked {
		return
	}
	if status != store.DiscordGuildApproved {
		h.requestGuildAccess(ctx, request, status)
		return
	}

	allowed, _ := h.perUser.Allow(request.source.UserID)
	var reply message
	switch {
	case !allowed:
		reply = plain("Too many requests. Try again in a minute.")
	case request.prompt == "" || len([]rune(request.prompt)) > maxPromptLength:
		reply = plain("Mention me with a request of up to 1,500 characters.")
	default:
		work, cancel := context.WithTimeout(ctx, 45*time.Second)
		var err error
		reply, err = h.answer(work, request.source, request.channelID, request.messageID, request.userName, request.prompt)
		if err != nil {
			reply = plain(h.userError(work, "answer Discord mention", err))
		}
		cancel()
		// Avoid sending a reply from work that was in flight when access was revoked.
		status, err = h.guildAccess(ctx, request.source)
		if err != nil || status != store.DiscordGuildApproved {
			return
		}
	}

	h.deliverMention(ctx, request, reply)
}

func (h *Bot) deliverMention(ctx context.Context, request mention, reply message) {
	delivery, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := h.sendReply(delivery, request, reply); err != nil && ctx.Err() == nil {
		h.logger.WarnContext(delivery, "reply to Discord mention", "error", err)
	}
}
