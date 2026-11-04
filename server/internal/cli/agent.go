package cli

import (
	"fmt"
	"strings"
)

// detectedSource is the source chooseAgent reports for an agent found in the environment.
const detectedSource = "detected from the environment"

// Agent names the CLI can act as for a signed-in person.
const (
	agentClaudeCode = "claude-code"
	agentCodex      = "codex"
)

// detectAgent returns the coding agent the CLI runs inside, or "" for a person at a terminal.
// Claude Code sets CLAUDECODE=1 in the commands it runs. Codex sets CODEX_SANDBOX and
// CODEX_SANDBOX_NETWORK_DISABLED in sandboxed commands and CODEX_THREAD_ID in its sessions.
// Claude Code is checked first because it is the only one that sets a value to compare.
func detectAgent(getenv func(string) string) string {
	if getenv("CLAUDECODE") == "1" {
		return agentClaudeCode
	}
	for _, name := range []string{"CODEX_SANDBOX", "CODEX_SANDBOX_NETWORK_DISABLED", "CODEX_THREAD_ID"} {
		if getenv(name) != "" {
			return agentCodex
		}
	}
	return ""
}

// chooseAgent picks the agent to act as: the --agent flag, then SPARROW_AGENT, then detection.
// An explicit value that is not a known agent is an error. source says where the choice came from.
func chooseAgent(flagValue string, getenv func(string) string) (name, source string, err error) {
	explicit, source := strings.TrimSpace(flagValue), "--agent"
	if explicit == "" {
		explicit, source = strings.TrimSpace(getenv("SPARROW_AGENT")), "SPARROW_AGENT"
	}
	if explicit != "" {
		if explicit != agentClaudeCode && explicit != agentCodex {
			return "", "", fmt.Errorf("%s must be %s or %s, not %q", source, agentClaudeCode, agentCodex, explicit)
		}
		return explicit, source, nil
	}

	if detected := detectAgent(getenv); detected != "" {
		return detected, detectedSource, nil
	}
	return "", "", nil
}
