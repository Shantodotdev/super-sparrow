package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
)

// callAgentTool forwards validated JSON arguments to any tool without duplicating its schema.
func (r *runner) callAgentTool(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("tool call", flag.ContinueOnError)
	inline := flags.String("arguments", "", "JSON arguments")
	file := flags.String("arguments-file", "", "JSON arguments file, or - for stdin")
	positional, err := parse(flags, args, 1)
	if err != nil {
		return nil, err
	}
	text, err := r.text("arguments", *inline, *file, true)
	if err != nil {
		return nil, err
	}
	var arguments map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &arguments); err != nil || arguments == nil {
		return nil, usageError("arguments must be a JSON object")
	}
	return r.agentTool(ctx, "tools/call", map[string]any{"name": positional[0], "arguments": arguments})
}

// agentTool returns structured MCP output and turns protocol and tool failures into CLI errors.
func (r *runner) agentTool(ctx context.Context, method string, params any) (json.RawMessage, error) {
	data, err := r.client.do(ctx, http.MethodPost, "/mcp", nil, map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode MCP response: %w", err)
	}
	if envelope.Error != nil {
		return nil, errors.New(envelope.Error.Message)
	}
	if method == "tools/list" {
		return envelope.Result, nil
	}
	var result struct {
		IsError           bool            `json:"isError"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		Content           []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, fmt.Errorf("decode tool result: %w", err)
	}
	if result.IsError {
		if len(result.Content) > 0 {
			return nil, errors.New(result.Content[0].Text)
		}
		return nil, errors.New("tool failed")
	}
	if len(result.StructuredContent) > 0 {
		return result.StructuredContent, nil
	}
	if len(result.Content) > 0 && json.Valid([]byte(result.Content[0].Text)) {
		return json.RawMessage(result.Content[0].Text), nil
	}
	return nil, errors.New("tool returned no JSON result")
}
