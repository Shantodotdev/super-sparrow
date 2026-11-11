package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"
)

// mcpServerName is the name Super Sparrow is registered under in each agent.
const mcpServerName = "super-sparrow"

// agentClient is a coding agent that setup can connect, and the commands that register the
// bridge with it at user scope.
type agentClient struct {
	name    string
	label   string
	program string
	remove  []string
	add     func(executable string) []string
}

var agentClients = []agentClient{
	{
		name: agentClaudeCode, label: "Claude Code", program: "claude",
		remove: []string{"mcp", "remove", "--scope", "user", mcpServerName},
		add: func(executable string) []string {
			return []string{"mcp", "add", "--scope", "user", mcpServerName, "--", executable, "--agent", agentClaudeCode, "mcp"}
		},
	},
	{
		name: agentCodex, label: "Codex", program: "codex",
		remove: []string{"mcp", "remove", mcpServerName},
		add: func(executable string) []string {
			return []string{"mcp", "add", mcpServerName, "--", executable, "--agent", agentCodex, "mcp"}
		},
	},
}

// setup signs in when needed and connects the installed coding agents to Super Sparrow through
// `sparrow mcp`, so no token is created or pasted. It is safe to run again.
func (r *runner) setup(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	rawURL := flags.String("url", "", "")
	emailAddress := flags.String("email", "", "")
	noOpen := flags.Bool("no-open", false, "")
	agentList := flags.String("agents", "", "")
	_, err := parse(flags, args, 0)
	if err != nil {
		return err
	}
	flags.Visit(func(set *flag.Flag) {
		if set.Name == "agents" && strings.TrimSpace(*agentList) == "" {
			err = usageError("--agents needs claude-code, codex, or both")
		}
	})
	if err != nil {
		return err
	}
	chosen, err := r.chooseAgentClients(*agentList)
	if err != nil {
		return err
	}
	executable, err := r.env.Executable()
	if err != nil {
		return fmt.Errorf("find this program's path: %w", err)
	}

	// Sign in unless a working login is already stored.
	if err := r.ensureLogin(ctx, *rawURL, *emailAddress, *noOpen); err != nil {
		return err
	}

	// Register the bridge with each agent, replacing an earlier registration.
	var failures []error
	for _, agent := range chosen {
		if err := r.connectAgent(ctx, agent, executable); err != nil {
			fmt.Fprintf(r.out, "✗ %s: %v\n", agent.label, err)
			failures = append(failures, fmt.Errorf("%s: %w", agent.label, err))
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	fmt.Fprintln(r.out, "Done. Restart any agent that is already running so it picks up Super Sparrow.")
	return nil
}

// chooseAgentClients returns the agents named in list, or every installed agent when list is
// empty. A named agent that is not installed is an error.
func (r *runner) chooseAgentClients(list string) ([]agentClient, error) {
	if strings.TrimSpace(list) == "" {
		var installed []agentClient
		for _, agent := range agentClients {
			if _, err := r.env.LookPath(agent.program); err == nil {
				installed = append(installed, agent)
			}
		}
		if len(installed) == 0 {
			return nil, errors.New("neither Claude Code (claude) nor Codex (codex) is on PATH; install one, then run sparrow setup again")
		}
		return installed, nil
	}

	var chosen []agentClient
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		index := -1
		for i, agent := range agentClients {
			if agent.name == name {
				index = i
			}
		}
		if index < 0 {
			return nil, usageError(fmt.Sprintf("--agents takes %s and %s, not %q", agentClaudeCode, agentCodex, name))
		}
		agent := agentClients[index]
		if _, err := r.env.LookPath(agent.program); err != nil {
			return nil, fmt.Errorf("%s is not installed: %s is not on PATH", agent.label, agent.program)
		}
		chosen = append(chosen, agent)
	}
	return chosen, nil
}

// ensureLogin keeps a stored login that the server still accepts for the same server, and signs
// in otherwise.
func (r *runner) ensureLogin(ctx context.Context, rawURL, emailAddress string, noOpen bool) error {
	creds, err := r.env.store().load()
	if err == nil {
		wanted, urlErr := r.env.serverURL(rawURL)
		sameServer := urlErr != nil || wanted.String() == creds.URL
		if baseURL, parseErr := parseServerURL(creds.URL); parseErr == nil && sameServer {
			if _, err := newClient(baseURL, creds.Token, "", "").do(ctx, http.MethodGet, "/api/me", nil, nil); err == nil {
				fmt.Fprintf(r.errOut, "Using the stored login for %s.\n", baseURL)
				return nil
			}
		}
	}

	loginArgs := []string{}
	if rawURL != "" {
		loginArgs = append(loginArgs, "--url", rawURL)
	}
	if emailAddress != "" {
		loginArgs = append(loginArgs, "--email", emailAddress)
	}
	if noOpen {
		loginArgs = append(loginArgs, "--no-open")
	}
	return r.login(ctx, loginArgs)
}

// connectAgent registers the bridge with one agent and checks that its tools answer as that
// agent, without changing any project data.
func (r *runner) connectAgent(ctx context.Context, agent agentClient, executable string) error {
	// Removing fails when nothing was registered yet, which is fine.
	_, _ = r.env.Command(ctx, agent.program, agent.remove...)
	if output, err := r.env.Command(ctx, agent.program, agent.add(executable)...); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", agent.program, strings.Join(agent.add(executable)[:2], " "), err, strings.TrimSpace(string(output)))
	}

	conn, err := r.env.connect(agent.name)
	if err != nil {
		return err
	}
	check := &runner{env: r.env, conn: conn, client: conn.client, out: r.out, errOut: r.errOut}
	tools, err := check.agentTool(ctx, "tools/list", map[string]any{})
	if err != nil {
		return fmt.Errorf("check the connection: %w", err)
	}
	var listed struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(tools, &listed); err != nil {
		return fmt.Errorf("read the tool list: %w", err)
	}
	me, err := conn.client.do(ctx, http.MethodGet, "/api/me", nil, nil)
	if err != nil {
		return fmt.Errorf("check who the agent acts as: %w", err)
	}
	var actor struct {
		Actor struct {
			DisplayName string `json:"display_name"`
		} `json:"actor"`
	}
	if err := json.Unmarshal(me, &actor); err != nil {
		return fmt.Errorf("read who the agent acts as: %w", err)
	}

	fmt.Fprintf(r.out, "✓ %s connected as %s, with %d tools.\n", agent.label, actor.Actor.DisplayName, len(listed.Tools))
	return nil
}
