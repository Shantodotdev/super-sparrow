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
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
)

// childGracePeriod is how long `sparrow run` waits for its child to stop after forwarding a
// termination request before killing it.
const childGracePeriod = 10 * time.Second

// secretsPath returns the API path of a project's secrets, with optional extra segments.
func secretsPath(project string, segments ...string) string {
	path := "/api/projects/" + url.PathEscape(project) + "/secrets"
	for _, segment := range segments {
		path += "/" + url.PathEscape(segment)
	}
	return path
}

// secrets runs the secrets subcommands.
func (r *runner) secrets(ctx context.Context, args []string) (json.RawMessage, error) {
	if len(args) == 0 {
		return nil, usageError("secrets needs a subcommand: list, download, set, unset, or import")
	}

	rest := args[1:]
	switch args[0] {
	case "list":
		return r.secretsList(ctx, rest)
	case "download":
		return nil, r.secretsDownload(ctx, rest)
	case "set":
		return r.secretsChange(ctx, "set", rest)
	case "unset":
		return r.secretsChange(ctx, "unset", rest)
	case "import":
		return r.secretsImport(ctx, rest)
	}
	return nil, usageError("unknown command secrets " + args[0])
}

// secretsList shows the secret names of every config of a project, and the required names that
// have no value.
func (r *runner) secretsList(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("secrets list", flag.ContinueOnError)
	project := flags.String("project", "", "")
	if _, err := parse(flags, args, 0); err != nil {
		return nil, err
	}
	if *project == "" {
		return nil, usageError("secrets list needs --project")
	}

	data, err := r.client.do(ctx, http.MethodGet, secretsPath(*project), nil, nil)
	if err != nil {
		return nil, err
	}
	if r.asJSON {
		return data, nil
	}

	var listing struct {
		Enabled bool `json:"enabled"`
		Configs map[string]struct {
			Secrets []struct {
				Name   string `json:"name"`
				Source string `json:"source"`
			} `json:"secrets"`
			Missing []string `json:"missing"`
		} `json:"configs"`
	}
	if err := json.Unmarshal(data, &listing); err != nil {
		return nil, fmt.Errorf("decode secrets: %w", err)
	}
	if !listing.Enabled {
		fmt.Fprintf(r.out, "Secrets are not enabled for %s.\n", *project)
		return nil, nil
	}

	for _, name := range []string{"dev", "dev-defaults", "staging", "production"} {
		config, found := listing.Configs[name]
		if !found {
			continue
		}
		fmt.Fprintf(r.out, "%s:\n", name)
		if len(config.Secrets) == 0 {
			fmt.Fprintln(r.out, "  (none)")
		}
		for _, secret := range config.Secrets {
			suffix := ""
			if secret.Source != "" {
				suffix = " (" + secret.Source + ")"
			}
			fmt.Fprintf(r.out, "  %s%s\n", secret.Name, suffix)
		}
		if len(config.Missing) > 0 {
			fmt.Fprintf(r.out, "  missing: %s\n", strings.Join(config.Missing, ", "))
		}
	}
	return nil, nil
}

// secretsDownload prints a config's values in the requested format.
func (r *runner) secretsDownload(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("secrets download", flag.ContinueOnError)
	project := flags.String("project", "", "")
	config := flags.String("config", "dev", "")
	format := flags.String("format", "json", "")
	if _, err := parse(flags, args, 0); err != nil {
		return err
	}
	if *project == "" {
		return usageError("secrets download needs --project")
	}
	if *format != "json" && *format != "env" && *format != "shell" {
		return usageError("--format must be json, env, or shell")
	}

	body, err := r.client.request(ctx, http.MethodGet, secretsPath(*project, *config, "download"), url.Values{"format": {*format}}, nil)
	if err != nil {
		return err
	}
	if len(body) > 0 && !bytes.HasSuffix(body, []byte("\n")) {
		body = append(body, '\n')
	}
	_, err = r.out.Write(body)
	return err
}

// maxSecretValueSize is the longest secret value the server accepts, 64 KiB.
const maxSecretValueSize = 64 << 10

// secretsChange sets or unsets the caller's dev overrides. A set argument is NAME=VALUE, or a bare
// NAME whose value is read from standard input, which keeps it out of the process list and the
// shell history.
func (r *runner) secretsChange(ctx context.Context, verb string, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("secrets "+verb, flag.ContinueOnError)
	project := flags.String("project", "", "")
	positional, err := parse(flags, args, anyCount)
	if err != nil {
		return nil, err
	}
	if *project == "" || len(positional) == 0 {
		return nil, usageError(fmt.Sprintf("secrets %s needs --project and at least one name", verb))
	}

	body := map[string]any{}
	if verb == "set" {
		values := make(map[string]string, len(positional))
		stdinName := ""
		for _, argument := range positional {
			name, value, found := strings.Cut(argument, "=")
			if !secretNamePattern.MatchString(name) {
				return nil, usageError(fmt.Sprintf("%q must start with an upper-case NAME, as in NAME=VALUE or just NAME", name))
			}
			if !found {
				if stdinName != "" {
					return nil, usageError("only one NAME can take its value from standard input; give the others as NAME=VALUE")
				}
				stdinName = name
			}
			values[name] = value
		}

		if stdinName != "" {
			value, err := r.readSecretValue(stdinName)
			if err != nil {
				return nil, err
			}
			values[stdinName] = value
		}
		body["set"] = values
	} else {
		for _, name := range positional {
			if !secretNamePattern.MatchString(name) {
				return nil, usageError(fmt.Sprintf("%q is not a valid secret name", name))
			}
		}
		body["delete"] = positional
	}
	return r.client.do(ctx, http.MethodPatch, secretsPath(*project, "dev"), nil, body)
}

// readSecretValue reads one secret value from standard input and drops a single trailing newline.
// On a terminal it prompts first; the standard library cannot turn echo off, so the prompt says
// the typing is visible.
func (r *runner) readSecretValue(name string) (string, error) {
	if isTerminal(r.stdin) {
		fmt.Fprintf(r.errOut, "Value for %s (what you type is shown; pipe it in to avoid that): ", name)
		line, err := bufio.NewReader(r.stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read the value of %s: %w", name, err)
		}
		return trimOneNewline(line), nil
	}

	data, err := io.ReadAll(io.LimitReader(r.stdin, maxSecretValueSize+2))
	if err != nil {
		return "", fmt.Errorf("read the value of %s from standard input: %w", name, err)
	}
	value := trimOneNewline(string(data))
	if len(value) > maxSecretValueSize {
		return "", fmt.Errorf("the value of %s is larger than %d bytes", name, maxSecretValueSize)
	}
	return value, nil
}

// trimOneNewline removes a single trailing line ending (LF or CRLF) from text.
func trimOneNewline(text string) string {
	if trimmed, ok := strings.CutSuffix(text, "\n"); ok {
		return strings.TrimSuffix(trimmed, "\r")
	}
	return text
}

// isTerminal reports whether reader is an interactive terminal rather than a pipe or a file.
func isTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// secretsImport sets the caller's dev overrides from a .env file.
func (r *runner) secretsImport(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("secrets import", flag.ContinueOnError)
	project := flags.String("project", "", "")
	file := flags.String("file", "", "")
	if _, err := parse(flags, args, 0); err != nil {
		return nil, err
	}
	if *project == "" || *file == "" {
		return nil, usageError("secrets import needs --project and --file (- for standard input)")
	}

	reader := r.stdin
	if *file != "-" {
		opened, err := os.Open(*file)
		if err != nil {
			return nil, fmt.Errorf("open --file: %w", err)
		}
		defer opened.Close()
		reader = opened
	}

	values, err := parseDotEnv(reader)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", *file, err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s holds no NAME=value lines", *file)
	}
	return r.client.do(ctx, http.MethodPatch, secretsPath(*project, "dev"), nil, map[string]any{"set": values})
}

// runWithSecrets runs a command with the project's dev secrets added to its environment. A secret
// replaces a variable of the same name. The command's exit status becomes this program's.
func (r *runner) runWithSecrets(ctx context.Context, args []string) error {
	separator := -1
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator == len(args)-1 {
		return usageError("run needs a command after --, such as: sparrow run --project WP -- npm start")
	}

	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	project := flags.String("project", "", "")
	if _, err := parse(flags, args[:separator], 0); err != nil {
		return err
	}
	if *project == "" {
		return usageError("run needs --project")
	}

	body, err := r.client.request(ctx, http.MethodGet, secretsPath(*project, "dev", "download"), url.Values{"format": {"json"}}, nil)
	if err != nil {
		return err
	}
	var values map[string]string
	if err := json.Unmarshal(body, &values); err != nil {
		return fmt.Errorf("decode secrets: %w", err)
	}

	command := args[separator+1:]
	child := exec.CommandContext(ctx, command[0], command[1:]...)
	child.Env = mergeEnvironment(os.Environ(), values)
	child.Stdin, child.Stdout, child.Stderr = r.stdin, r.out, r.errOut

	// When this program is told to stop (SIGTERM, or Ctrl-C), the child is sent SIGTERM (killed on
	// Windows) and given a grace period before it is killed. On Ctrl-C the child has also received
	// the terminal's SIGINT itself.
	child.Cancel = func() error { return terminateProcess(child.Process) }
	child.WaitDelay = childGracePeriod

	err = child.Run()
	if err == nil {
		return nil
	}

	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return &ExitError{Code: exitStatus(exit)}
	case ctx.Err() != nil && errors.Is(err, ctx.Err()):
		// The child stopped cleanly after being told to; report it as terminated, like a signal.
		return &ExitError{Code: 128 + int(syscall.SIGTERM)}
	}
	return fmt.Errorf("run %s: %w", command[0], err)
}

// mergeEnvironment returns base (NAME=value entries) with values added; values win.
func mergeEnvironment(base []string, values map[string]string) []string {
	merged := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[name]; !replaced {
			merged = append(merged, entry)
		}
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		merged = append(merged, name+"="+values[name])
	}
	return merged
}
