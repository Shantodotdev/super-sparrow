package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// toolSpec is an agent tool as tools/list describes it.
type toolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema struct {
		Properties map[string]toolProperty `json:"properties"`
		Required   []string                `json:"required"`
	} `json:"inputSchema"`
}

// toolProperty is one argument of a tool.
type toolProperty struct {
	Type        json.RawMessage `json:"type"`
	Description string          `json:"description"`
	Items       *struct {
		Type json.RawMessage `json:"type"`
	} `json:"items"`
}

// jsonType returns the first non-null JSON Schema type in raw, which is a name or a list of names.
func jsonType(raw json.RawMessage) string {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single
	}

	var several []string
	if json.Unmarshal(raw, &several) == nil {
		for _, name := range several {
			if name != "null" {
				return name
			}
		}
	}
	return ""
}

// kind returns how a flag's value is read: string, integer, number, boolean, string-list, or json.
func (p toolProperty) kind() string {
	switch kind := jsonType(p.Type); kind {
	case "string", "integer", "number", "boolean":
		return kind
	case "array":
		if p.Items != nil && jsonType(p.Items.Type) == "string" {
			return "string-list"
		}
	}
	return "json"
}

// errToolHelp tells the caller that --help was asked for.
var errToolHelp = errors.New("help requested")

// fetchTools returns the tools the server offers, sorted by name.
func (r *runner) fetchTools(ctx context.Context) ([]toolSpec, error) {
	data, err := r.agentTool(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var listed struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		return nil, fmt.Errorf("decode the tool list: %w", err)
	}
	sort.Slice(listed.Tools, func(i, j int) bool { return listed.Tools[i].Name < listed.Tools[j].Name })
	return listed.Tools, nil
}

// listTools prints each tool's name and description.
func (r *runner) listTools(ctx context.Context, args []string) (json.RawMessage, error) {
	if _, err := parse(flag.NewFlagSet("tools", flag.ContinueOnError), args, 0); err != nil {
		return nil, err
	}

	tools, err := r.fetchTools(ctx)
	if err != nil {
		return nil, err
	}
	if r.asJSON {
		return json.Marshal(map[string]any{"tools": tools})
	}

	width := 0
	for _, tool := range tools {
		width = max(width, len(tool.Name))
	}
	for _, tool := range tools {
		fmt.Fprintf(r.out, "%-*s  %s\n", width, tool.Name, firstLine(tool.Description))
	}
	return nil, nil
}

// firstLine returns the first line of text.
func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// normalizeName makes tool and flag names comparable whether they use underscores or dashes.
func normalizeName(name string) string { return strings.ReplaceAll(name, "-", "_") }

// callTool runs the agent tool called name, building its arguments from flags.
func (r *runner) callTool(ctx context.Context, name string, args []string) (json.RawMessage, error) {
	tools, err := r.fetchTools(ctx)
	if err != nil {
		return nil, err
	}

	var spec *toolSpec
	for i := range tools {
		if normalizeName(tools[i].Name) == normalizeName(name) {
			spec = &tools[i]
			break
		}
	}
	if spec == nil {
		return nil, usageError(fmt.Sprintf("unknown command %q; `sparrow tools` lists the agent tools", name))
	}

	arguments, err := buildArguments(spec, args)
	if errors.Is(err, errToolHelp) {
		_, err := io.WriteString(r.out, toolHelp(spec))
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return r.agentTool(ctx, "tools/call", map[string]any{"name": spec.Name, "arguments": arguments})
}

// buildArguments turns command-line flags into the arguments of a tool, reading each flag's value
// by the type its input schema gives. It returns errToolHelp when --help is among the flags.
func buildArguments(spec *toolSpec, args []string) (map[string]any, error) {
	byFlag := make(map[string]string, len(spec.InputSchema.Properties))
	for name := range spec.InputSchema.Properties {
		byFlag[normalizeName(name)] = name
	}

	arguments := map[string]any{}
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			return nil, usageError(fmt.Sprintf("unexpected argument %q: tool arguments are flags such as --name value", arg))
		}
		if arg == "-h" {
			return nil, errToolHelp
		}
		if !strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "---") {
			return nil, usageError(fmt.Sprintf("invalid flag %q: tool flags start with exactly two dashes, such as --name value", arg))
		}

		flagName, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		property, found := byFlag[normalizeName(flagName)]
		if !found {
			if flagName == "help" {
				return nil, errToolHelp
			}
			return nil, usageError(fmt.Sprintf("unknown flag --%s for %s; valid flags: %s", flagName, spec.Name, flagList(spec)))
		}

		def := spec.InputSchema.Properties[property]
		kind := def.kind()
		if _, repeated := arguments[property]; repeated && kind != "string-list" {
			return nil, usageError(fmt.Sprintf("--%s was given more than once", flagName))
		}
		if kind == "boolean" && !hasValue && len(args) > 0 && (args[0] == "true" || args[0] == "false") {
			value, args, hasValue = args[0], args[1:], true
		}
		if kind != "boolean" && !hasValue {
			if len(args) == 0 {
				return nil, usageError(fmt.Sprintf("--%s needs a value", flagName))
			}
			value, args = args[0], args[1:]
		}

		parsed, err := parseFlagValue(kind, value, hasValue)
		if err != nil {
			return nil, usageError(fmt.Sprintf("--%s: %v", flagName, err))
		}
		if kind == "string-list" {
			list, ok := arguments[property].([]string)
			if !ok {
				list = []string{}
			}
			parsed = append(list, parsed.([]string)...)
		}
		arguments[property] = parsed
	}

	var missing []string
	for _, name := range spec.InputSchema.Required {
		if _, given := arguments[name]; !given {
			missing = append(missing, "--"+name)
		}
	}
	if len(missing) > 0 {
		return nil, usageError(fmt.Sprintf("%s needs %s", spec.Name, strings.Join(missing, ", ")))
	}
	return arguments, nil
}

// parseFlagValue reads one flag value as the given kind. A boolean flag without a value is true.
func parseFlagValue(kind, value string, hasValue bool) (any, error) {
	switch kind {
	case "boolean":
		if !hasValue {
			return true, nil
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("%q is not true or false", value)
		}
		return parsed, nil
	case "integer":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a whole number", value)
		}
		return parsed, nil
	case "number":
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", value)
		}
		return parsed, nil
	case "string-list":
		items := []string{}
		for item := range strings.SplitSeq(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				items = append(items, item)
			}
		}
		return items, nil
	case "json":
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, fmt.Errorf("%q is not valid JSON", value)
		}
		return parsed, nil
	}
	return value, nil
}

// flagList lists a tool's flags for an error message.
func flagList(spec *toolSpec) string {
	names := make([]string, 0, len(spec.InputSchema.Properties))
	for name := range spec.InputSchema.Properties {
		names = append(names, "--"+name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// toolHelp describes a tool and its flags, marking the required ones.
func toolHelp(spec *toolSpec) string {
	required := make(map[string]bool, len(spec.InputSchema.Required))
	for _, name := range spec.InputSchema.Required {
		required[name] = true
	}

	names := make([]string, 0, len(spec.InputSchema.Properties))
	for name := range spec.InputSchema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", spec.Name)
	if description := strings.TrimSpace(spec.Description); description != "" {
		fmt.Fprintf(&b, "\n%s\n", description)
	}
	if len(names) == 0 {
		b.WriteString("\nThis tool takes no flags.\n")
		return b.String()
	}

	b.WriteString("\nFlags:\n")
	for _, name := range names {
		property := spec.InputSchema.Properties[name]
		marker := ""
		if required[name] {
			marker = " (required)"
		}
		fmt.Fprintf(&b, "  --%s <%s>%s\n", name, property.kind(), marker)
		if description := strings.TrimSpace(property.Description); description != "" {
			for line := range strings.SplitSeq(description, "\n") {
				fmt.Fprintf(&b, "      %s\n", line)
			}
		}
	}
	return b.String()
}
