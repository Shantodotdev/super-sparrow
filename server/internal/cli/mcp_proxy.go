package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// maxMCPMessage is the largest JSON-RPC message the bridge reads from the agent, matching the
// server's request limit.
const maxMCPMessage = 1 << 20

// JSON-RPC codes for a request the bridge could not deliver, and one it refused to read.
const (
	jsonRPCInternalError  = -32603
	jsonRPCInvalidRequest = -32600
)

// serveMCP runs a local MCP server over standard input and output for an agent such as Claude
// Code. Each JSON-RPC message from the agent is forwarded to the server's /mcp endpoint with the
// stored login acting as that agent, so no token is ever configured in the agent.
func (r *runner) serveMCP(ctx context.Context, args []string) error {
	if _, err := parse(flag.NewFlagSet("mcp", flag.ContinueOnError), args, 0); err != nil {
		return err
	}
	if r.conn.kind == authLogin && r.conn.agent == "" {
		return usageError("mcp needs the agent it serves: sparrow --agent claude-code mcp, or sparrow --agent codex mcp")
	}

	bridge := &mcpBridge{client: r.client, out: r.out}
	reader := bufio.NewReaderSize(r.stdin, 64<<10)
	var inFlight sync.WaitGroup
	defer inFlight.Wait()

	for {
		line, err := readMessage(reader)
		if errors.Is(err, errMessageTooLarge) {
			// The rest of the line was skipped, so the bridge keeps serving the next message.
			bridge.write(errorReply(json.RawMessage("null"), jsonRPCInvalidRequest, err.Error()))
			continue
		}
		if len(bytes.TrimSpace(line)) > 0 {
			// Requests run side by side, so a slow tool call does not hold up the others.
			inFlight.Add(1)
			go func(message []byte) {
				defer inFlight.Done()
				bridge.forward(ctx, message)
			}(line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read from the agent: %w", err)
		}
	}
}

// errMessageTooLarge means a message was longer than maxMCPMessage and was skipped.
var errMessageTooLarge = fmt.Errorf("the message is longer than %d bytes and was skipped", maxMCPMessage)

// readMessage reads one newline-delimited message. A message longer than maxMCPMessage is read
// to its end and dropped, and errMessageTooLarge is returned.
func readMessage(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	tooLarge := false
	for {
		chunk, err := reader.ReadSlice('\n')
		if !tooLarge {
			line = append(line, chunk...)
			if len(line) > maxMCPMessage {
				tooLarge, line = true, nil
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if tooLarge && err == nil {
			return nil, errMessageTooLarge
		}
		return line, err
	}
}

// mcpBridge forwards messages to the server and writes its answers back one line at a time.
type mcpBridge struct {
	client *client
	out    io.Writer
	mu     sync.Mutex
}

// forward sends one message and writes the server's answer. Notifications get no answer. When
// the server cannot be reached for a request, the agent gets a JSON-RPC error for that request
// instead of waiting forever.
func (b *mcpBridge) forward(ctx context.Context, message []byte) {
	var envelope struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(message, &envelope); err != nil {
		b.write(errorReply(json.RawMessage("null"), -32700, "the message is not valid JSON"))
		return
	}

	data, err := b.client.request(ctx, http.MethodPost, "/mcp", nil, json.RawMessage(message))
	if err != nil {
		if len(envelope.ID) == 0 {
			return
		}
		var failure *apiError
		text := err.Error()
		if errors.As(err, &failure) && failure.Status == http.StatusUnauthorized {
			text = "Super Sparrow rejected the stored login; run `sparrow login` in a terminal, then restart the agent"
		}
		b.write(errorReply(envelope.ID, jsonRPCInternalError, text))
		return
	}

	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		b.write(errorReply(envelope.ID, jsonRPCInternalError, "the server sent an answer that is not JSON"))
		return
	}
	b.write(compact.Bytes())
}

// write sends one message to the agent. Messages from concurrent requests never interleave.
func (b *mcpBridge) write(message []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.out.Write(append(message, '\n'))
}

func errorReply(id json.RawMessage, code int, message string) []byte {
	reply, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message},
	})
	return reply
}
