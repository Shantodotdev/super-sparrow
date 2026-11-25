package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/mcp"
)

const anthropicMessages = "https://api.anthropic.com/v1/messages"

type modelClient interface {
	complete(context.Context, string, []conversation, []mcp.Definition) ([]json.RawMessage, error)
}

type conversation struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type cacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type systemBlock struct {
	Type         string       `json:"type"`
	Text         string       `json:"text"`
	CacheControl cacheControl `json:"cache_control"`
}

var promptCache = cacheControl{Type: "ephemeral", TTL: "1h"}

type anthropicClient struct {
	key      string
	model    string
	client   *http.Client
	endpoint string
}

func (a anthropicClient) complete(ctx context.Context, system string, messages []conversation, tools []mcp.Definition) ([]json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{
		"model":         a.model,
		"max_tokens":    2048,
		"cache_control": promptCache,
		"system":        []systemBlock{{Type: "text", Text: system, CacheControl: promptCache}},
		"messages":      messages,
		"tools":         tools,
		"output_config": map[string]string{"effort": "low"},
		"tool_choice":   map[string]any{"type": "auto", "disable_parallel_tool_use": true},
	})
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build Anthropic request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", a.key)
	request.Header.Set("anthropic-version", "2023-06-01")

	reply, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Anthropic Messages API: %w", err)
	}
	defer reply.Body.Close()
	if reply.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("call Anthropic Messages API: HTTP %d", reply.StatusCode)
	}

	var result struct {
		Content []json.RawMessage `json:"content"`
		Stop    string            `json:"stop_reason"`
	}
	if err := json.NewDecoder(io.LimitReader(reply.Body, 128<<10)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Anthropic response: %w", err)
	}
	if result.Stop != "tool_use" && result.Stop != "end_turn" {
		return nil, fmt.Errorf("model stopped without a complete answer")
	}
	return result.Content, nil
}
