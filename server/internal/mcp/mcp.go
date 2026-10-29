// Package mcp serves Super Sparrow's agent tools over the Model Context Protocol's streamable
// HTTP transport.
//
// The server is stateless: every POST carries one JSON-RPC message, or a batch, and is answered
// with JSON. It offers no server-to-client stream, so GET is refused, and it keeps no sessions.
// The HTTP layer authenticates the agent token and passes in the caller, so every tool call is
// recorded as the agent that made it.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// Protocol versions this server speaks, newest first.
var protocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

const (
	maxBodyBytes = 1 << 20

	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

const instructions = `Super Sparrow tracks this team's issues, plans, and code reviews. Each issue has a key such as WP-142, and its description is its plan.

Start from the current git branch: get_issue_for_branch finds the issue, and get_plan returns the plan and its checklist. Tick checklist items with update_plan_checklist as you finish them.

To review: start_review_round, then add_finding for each problem, then finish_review_round with a summary. To address a review: list_open_findings shows what the developer decided to fix, with each finding's discussion, which may say how to fix it; after fixing, post_fix_summary marks those findings fixed and explains the fix. list_comments shows an issue's or a finding's discussion and the comment IDs to reply to with add_comment. Only a person decides whether a finding is fixed, won't be fixed, or is deferred.

Every change you make is recorded as your agent, owned by the person whose token you use.`

// Server answers MCP requests with the tools in tools.go.
type Server struct {
	store     *store.Store
	publicURL *url.URL
	logger    *slog.Logger
	version   string
	tools     []tool
}

// New returns a Server. publicURL is the address people open, used for links in tool results.
// version is reported to clients as the server's version.
func New(st *store.Store, publicURL *url.URL, logger *slog.Logger, version string) *Server {
	s := &Server{store: st, publicURL: publicURL, logger: logger, version: version}
	s.tools = s.toolList()
	return s
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeHTTP answers one POST from caller, who the HTTP layer authenticated from an agent token.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request, caller store.Caller) {
	if version := r.Header.Get("MCP-Protocol-Version"); version != "" && !slices.Contains(protocolVersions, version) {
		http.Error(w, "unsupported MCP-Protocol-Version "+version, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeResponse(w, errorResponse(nil, codeInvalidRequest, fmt.Sprintf("the body must be at most %d bytes", maxBodyBytes)))
		return
	}
	body = bytes.TrimSpace(body)

	// A batch is an array of messages; the reply holds the answers to its requests.
	if len(body) > 0 && body[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil || len(batch) == 0 {
			writeResponse(w, errorResponse(nil, codeParseError, "the body must be a JSON-RPC message or a non-empty batch"))
			return
		}

		var answers []response
		for _, raw := range batch {
			if answer, ok := s.handle(r.Context(), caller, raw); ok {
				answers = append(answers, answer)
			}
		}
		if len(answers) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeResponse(w, answers)
		return
	}

	answer, ok := s.handle(r.Context(), caller, body)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeResponse(w, answer)
}

// handle answers one message. It returns false for notifications and responses, which get no answer.
func (s *Server) handle(ctx context.Context, caller store.Caller, raw json.RawMessage) (response, bool) {
	var msg message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return errorResponse(nil, codeParseError, "the message is not valid JSON"), true
	}
	if msg.Method == "" {
		// A response to something the server sent; this server sends nothing, so ignore it.
		return response{}, false
	}
	if msg.JSONRPC != "2.0" {
		return errorResponse(msg.ID, codeInvalidRequest, `jsonrpc must be "2.0"`), msg.ID != nil
	}
	if msg.ID == nil {
		return response{}, false
	}

	switch msg.Method {
	case "initialize":
		return resultResponse(msg.ID, s.initialize(msg.Params)), true
	case "ping":
		return resultResponse(msg.ID, map[string]any{}), true
	case "tools/list":
		return resultResponse(msg.ID, map[string]any{"tools": s.toolDescriptions()}), true
	case "tools/call":
		result, rpcErr := s.callTool(ctx, caller, msg.Params)
		if rpcErr != nil {
			return response{JSONRPC: "2.0", ID: msg.ID, Error: rpcErr}, true
		}
		return resultResponse(msg.ID, result), true
	}
	return errorResponse(msg.ID, codeMethodNotFound, "unknown method "+msg.Method), true
}

// initialize agrees on a protocol version: the client's when this server speaks it, else the
// newest this server speaks, which the client may then refuse.
func (s *Server) initialize(params json.RawMessage) map[string]any {
	var request struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &request)

	version := protocolVersions[0]
	if slices.Contains(protocolVersions, request.ProtocolVersion) {
		version = request.ProtocolVersion
	}

	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "super-sparrow", "title": "Super Sparrow", "version": s.version},
		"instructions":    instructions,
	}
}

func (s *Server) toolDescriptions() []map[string]any {
	descriptions := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		descriptions = append(descriptions, map[string]any{
			"name": t.name, "title": t.title, "description": t.description, "inputSchema": t.schema,
			"annotations": map[string]any{"title": t.title, "readOnlyHint": t.readOnly, "destructiveHint": false, "openWorldHint": false},
		})
	}
	return descriptions
}

// callTool runs a tool. Problems with the tool's input or the change it asks for are reported as a
// tool result with isError set, so the agent can read and correct them; only an unknown tool or
// malformed params are protocol errors.
func (s *Server) callTool(ctx context.Context, caller store.Caller, params json.RawMessage) (map[string]any, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "params must be an object with name and arguments"}
	}

	index := slices.IndexFunc(s.tools, func(t tool) bool { return t.name == call.Name })
	if index < 0 {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool " + call.Name}
	}
	if len(call.Arguments) == 0 || string(call.Arguments) == "null" {
		call.Arguments = json.RawMessage("{}")
	}

	output, err := s.tools[index].run(ctx, caller, call.Arguments)
	if err != nil {
		return toolError(s.describeError(ctx, call.Name, err)), nil
	}

	text, err := json.Marshal(output)
	if err != nil {
		s.logger.ErrorContext(ctx, "encode tool result", "tool", call.Name, "error", err)
		return nil, &rpcError{Code: codeInternalError, Message: "internal error"}
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(text)}}, "isError": false}, nil
}

// describeError turns an error into the message the agent reads. Unexpected errors are logged and
// hidden, so internal details never reach the client.
func (s *Server) describeError(ctx context.Context, toolName string, err error) string {
	var validation *store.ValidationError
	switch {
	case errors.As(err, &validation):
		return "Invalid input: " + validation.Field + " " + validation.Message
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrForbidden),
		errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrInvalidTransition):
		return store.Explain(err)
	}

	s.logger.ErrorContext(ctx, "tool failed", "tool", toolName, "error", err)
	return "internal error; try again"
}

func toolError(message string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": message}}, "isError": true}
}

func resultResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) response {
	if id == nil {
		id = json.RawMessage("null")
	}
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func writeResponse(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
