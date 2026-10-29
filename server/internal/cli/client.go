// Package cli is the command-line client: it runs the same operations as the agent tools through
// the HTTP API, for scripts, hooks, and agents without MCP.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const requestTimeout = 30 * time.Second

// client calls the Super Sparrow API with an agent token.
type client struct {
	baseURL *url.URL
	token   string
	model   string
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

// newClient reads the client settings through getenv: SPARROW_URL and SPARROW_TOKEN are required,
// and SPARROW_MODEL, when set, is reported as the model making the changes.
func newClient(getenv func(string) string) (*client, error) {
	var problems []error

	rawURL := strings.TrimSpace(getenv("SPARROW_URL"))
	baseURL, err := url.Parse(rawURL)
	switch {
	case rawURL == "":
		problems = append(problems, errors.New("SPARROW_URL is required: the server's address, such as https://sparrow.example.com"))
	case err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "":
		problems = append(problems, errors.New("SPARROW_URL must be an http or https address"))
	}

	token := strings.TrimSpace(getenv("SPARROW_TOKEN"))
	if !strings.HasPrefix(token, "ssp_") {
		problems = append(problems, errors.New("SPARROW_TOKEN is required: an agent token from settings, starting with ssp_"))
	}

	if err := errors.Join(problems...); err != nil {
		return nil, err
	}
	return &client{
		baseURL: baseURL, token: token, model: strings.TrimSpace(getenv("SPARROW_MODEL")),
		http: &http.Client{Timeout: requestTimeout},
	}, nil
}

// do sends a request with an optional JSON body and returns the raw JSON response.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
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
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.model != "" {
		request.Header.Set("Sparrow-Model", c.model)
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
	if len(data) == 0 {
		return json.RawMessage("{}"), nil
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
