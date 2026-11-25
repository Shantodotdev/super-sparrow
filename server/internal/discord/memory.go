package discord

import (
	"slices"
	"strings"
	"time"
)

const (
	memoryLifetime    = 30 * time.Minute
	maxMemoryTurns    = 10
	maxMemoryChannels = 256
)

type memoryKey struct {
	guildID   string
	channelID string
}

type channelHistory struct {
	updatedAt time.Time
	messages  []conversation
}

// recentMessages returns a copy of non-expired channel history so callers can
// append tool-use turns without mutating shared memory.
func (h *Bot) recentMessages(guildID, channelID string) []conversation {
	h.memoryMu.Lock()
	defer h.memoryMu.Unlock()

	key := memoryKey{guildID: guildID, channelID: channelID}
	entry, ok := h.memory[key]
	if !ok {
		return nil
	}
	if h.now().Sub(entry.updatedAt) > memoryLifetime {
		delete(h.memory, key)
		return nil
	}
	return slices.Clone(entry.messages)
}

// rememberExchange appends one completed user/assistant turn to the channel's
// short-term memory, pruning expired channels and capping stored turns.
func (h *Bot) rememberExchange(guildID, channelID, userMessage, assistantMessage string) {
	userMessage = strings.TrimSpace(userMessage)
	assistantMessage = strings.TrimSpace(assistantMessage)
	if guildID == "" || channelID == "" || userMessage == "" || assistantMessage == "" {
		return
	}

	h.memoryMu.Lock()
	defer h.memoryMu.Unlock()

	now := h.now()
	for key, entry := range h.memory {
		if now.Sub(entry.updatedAt) > memoryLifetime {
			delete(h.memory, key)
		}
	}

	key := memoryKey{guildID: guildID, channelID: channelID}
	entry := h.memory[key]
	if _, exists := h.memory[key]; !exists && len(h.memory) >= maxMemoryChannels {
		h.evictOldestChannelLocked()
	}

	messages := append(entry.messages,
		conversation{Role: "user", Content: userMessage},
		conversation{Role: "assistant", Content: assistantMessage},
	)
	if limit := maxMemoryTurns * 2; len(messages) > limit {
		messages = slices.Clone(messages[len(messages)-limit:])
	}
	h.memory[key] = channelHistory{updatedAt: now, messages: messages}
}

func (h *Bot) clearGuildMemory(guildID string) {
	h.memoryMu.Lock()
	defer h.memoryMu.Unlock()

	for key := range h.memory {
		if key.guildID == guildID {
			delete(h.memory, key)
		}
	}
}

func (h *Bot) evictOldestChannelLocked() {
	var oldestKey memoryKey
	var oldestTime time.Time
	first := true
	for key, entry := range h.memory {
		if first || entry.updatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.updatedAt
			first = false
		}
	}
	if !first {
		delete(h.memory, oldestKey)
	}
}

func formatUserMessage(userName, prompt string) string {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return prompt
	}
	return userName + ": " + prompt
}
