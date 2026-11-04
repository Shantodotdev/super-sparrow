// Package cli is the command-line client: it runs the same operations as the agent tools through
// the HTTP API, for scripts, hooks, and agents without MCP.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const requestTimeout = 30 * time.Second

// client calls the Super Sparrow API with an agent token or a CLI session token.
type client struct {
	baseURL *url.URL
	token   string
	model   string
	agent   string
	http    *http.Client
}

// apiError is an error response from the API.
type apiError struct {
	Status  int
	Message string
	Field   string
}

func (e *apiError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Field, e.Message, e.Status)
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
}

// newClient makes a client for baseURL. An empty token sends no credentials (sign-in requests),
// and an empty agent acts as the signed-in person.
func newClient(baseURL *url.URL, token, model, agent string) *client {
	return &client{baseURL: baseURL, token: token, model: model, agent: agent, http: &http.Client{Timeout: requestTimeout}}
}

// do sends a request with an optional JSON body and returns the raw JSON response.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	data, err := c.request(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return json.RawMessage("{}"), nil
	}
	return data, nil
}

// request sends a request with an optional JSON body and returns the response body as it is.
func (c *client) request(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	target := c.baseURL.JoinPath(path)
	target.RawQuery = query.Encode()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json, text/plain")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.model != "" {
		request.Header.Set("Sparrow-Model", c.model)
	}
	if c.agent != "" {
		request.Header.Set("Sparrow-Agent", c.agent)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode >= 300 {
		failure := &apiError{Status: response.StatusCode, Message: strings.TrimSpace(string(data))}
		var decoded struct {
			Error string `json:"error"`
			Field string `json:"field"`
		}
		if json.Unmarshal(data, &decoded) == nil && decoded.Error != "" {
			failure.Message, failure.Field = decoded.Error, decoded.Field
		}
		return nil, failure
	}
	return data, nil
}

// issueID returns the ID of the issue with key.
func (c *client) issueID(ctx context.Context, key string) (string, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/issues/"+url.PathEscape(key), nil, nil)
	if err != nil {
		return "", err
	}

	var body struct {
		Issue struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", fmt.Errorf("read issue %s: %w", key, err)
	}
	return body.Issue.ID, nil
}
