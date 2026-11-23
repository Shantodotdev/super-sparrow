package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type messageReference struct {
	MessageID      string `json:"message_id"`
	FailIfNotExist bool   `json:"fail_if_not_exists"`
}

// sendReply posts a Discord reply, using the original message ID as a deduplication nonce.
func (h *Bot) sendReply(ctx context.Context, request mention, reply message) error {
	body, err := json.Marshal(struct {
		message
		Reference    messageReference `json:"message_reference"`
		Nonce        string           `json:"nonce"`
		EnforceNonce bool             `json:"enforce_nonce"`
	}{reply, messageReference{MessageID: request.messageID}, request.messageID, true})
	if err != nil {
		return fmt.Errorf("encode Discord reply: %w", err)
	}
	return h.postChannelMessage(ctx, request.channelID, body)
}

// postChannelMessage posts an encoded message to a channel, waiting once when Discord asks to retry.
func (h *Bot) postChannelMessage(ctx context.Context, channelID string, body []byte) error {
	endpoint := h.api + "/channels/" + channelID + "/messages"
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build Discord message: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bot "+h.cfg.BotToken)
		response, err := h.client.Do(req)
		if err != nil {
			return fmt.Errorf("post Discord message: %w", err)
		}
		response.Body.Close()
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
			return nil
		}
		retry, err := strconv.ParseFloat(response.Header.Get("Retry-After"), 64)
		if attempt > 0 || response.StatusCode != http.StatusTooManyRequests || err != nil || !(retry > 0 && retry <= 5) {
			return fmt.Errorf("post Discord message: HTTP %d", response.StatusCode)
		}
		timer := time.NewTimer(time.Duration(retry * float64(time.Second)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
