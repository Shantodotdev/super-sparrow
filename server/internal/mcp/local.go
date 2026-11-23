package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// Definition is the tool metadata used by an in-process client of the dispatcher.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Definitions returns schemas for the requested tools. Callers must treat the schemas as read-only.
func (s *Server) Definitions(names ...string) []Definition {
	var definitions []Definition
	for _, t := range s.tools {
		if slices.Contains(names, t.name) {
			definitions = append(definitions, Definition{t.name, t.description, t.schema})
		}
	}
	return definitions
}

// Call runs the same validated tool as the MCP transport for an already authenticated caller.
func (s *Server) Call(ctx context.Context, caller store.Caller, name string, arguments any) (json.RawMessage, bool, error) {
	params, err := json.Marshal(map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return nil, false, fmt.Errorf("encode arguments for %s: %w", name, err)
	}
	result, rpcErr := s.callTool(ctx, caller, params)
	if rpcErr != nil {
		return nil, false, fmt.Errorf("call %s: %s", name, rpcErr.Message)
	}
	content, err := json.Marshal(result["structuredContent"])
	if err != nil {
		return nil, false, fmt.Errorf("encode %s result: %w", name, err)
	}
	return content, result["isError"].(bool), nil
}
