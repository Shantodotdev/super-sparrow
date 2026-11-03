package cli

import (
	"errors"
	"fmt"
	"strings"
)

// authKind is how a connection is authorized.
type authKind string

const (
	authLogin authKind = "login"
	authToken authKind = "token"
)

// connection is a client together with how it was authorized, for whoami.
type connection struct {
	client      *client
	kind        authKind
	agent       string
	agentSource string
}

// connect builds the client for a command. SPARROW_TOKEN (an ssp_ agent token) with SPARROW_URL
// wins; otherwise the stored login is used and acts as the person or, inside Claude Code or Codex,
// as that person's agent. agentFlag is the --agent value.
func (e *Environment) connect(agentFlag string) (*connection, error) {
	agent, source, err := chooseAgent(agentFlag, e.Getenv)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(e.Getenv("SPARROW_MODEL"))

	if token := strings.TrimSpace(e.Getenv("SPARROW_TOKEN")); token != "" {
		if !strings.HasPrefix(token, "ssp_") {
			return nil, errors.New("SPARROW_TOKEN must be an agent token from settings, starting with ssp_")
		}
		baseURL, err := parseServerURL(e.Getenv("SPARROW_URL"))
		if err != nil {
			return nil, fmt.Errorf("SPARROW_URL: %w", err)
		}
		// An agent token is already an agent, so the agent header does not apply.
		if source != detectedSource {
			fmt.Fprintf(e.Stderr, "warning: %s is ignored because SPARROW_TOKEN is an agent token\n", source)
		}
		return &connection{client: newClient(baseURL, token, model, ""), kind: authToken}, nil
	}

	creds, err := e.store().load()
	if errors.Is(err, errNotSignedIn) {
		return nil, fmt.Errorf("%w: run `sparrow login --url URL`, or set SPARROW_URL and SPARROW_TOKEN (an ssp_ agent token)", errNotSignedIn)
	}
	if err != nil {
		return nil, err
	}
	baseURL, err := parseServerURL(creds.URL)
	if err != nil {
		return nil, fmt.Errorf("stored login: %w", err)
	}
	return &connection{client: newClient(baseURL, creds.Token, model, agent), kind: authLogin, agent: agent, agentSource: source}, nil
}

// store returns the credential store of this environment.
func (e *Environment) store() credentialStore {
	return credentialStore{keyring: e.Keyring, configDir: e.ConfigDir}
}
