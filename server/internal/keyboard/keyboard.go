// Package keyboard defines and validates the personal shortcut bindings shared with the web app.
package keyboard

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Definition describes an application action and its factory binding.
type Definition struct {
	Label   string `json:"label"`
	Group   string `json:"group"`
	Binding string `json:"binding"`
	Global  bool   `json:"global,omitempty"`
	Typing  bool   `json:"typing,omitempty"`
}

//go:embed definitions.json
var definitionsJSON []byte

// Definitions returns an independent catalog of supported application actions.
func Definitions() map[string]Definition {
	var definitions map[string]Definition
	if err := json.Unmarshal(definitionsJSON, &definitions); err != nil {
		panic(fmt.Sprintf("invalid built-in shortcut definitions: %v", err))
	}
	return definitions
}

var keyPattern = regexp.MustCompile(`^([A-Z0-9]|F([1-9]|1[0-9]|2[0-4])|Enter|Space|ArrowUp|ArrowDown|ArrowLeft|ArrowRight|Home|End|PageUp|PageDown|Backspace|Delete|Slash|Comma|Period|Semicolon|Quote|Backquote|BracketLeft|BracketRight|Backslash|Minus|Equal)$`)

// Validate checks canonical chords, reserved keys, global modifiers, and conflicting prefixes.
// An empty binding disables an action; absent overrides keep its built-in binding.
func Validate(overrides map[string]string) error {
	definitions := Definitions()
	// Validate supplied chords before comparing the full effective bindings.
	for id, binding := range overrides {
		definition, ok := definitions[id]
		if !ok {
			return fmt.Errorf("unknown action %q", id)
		}
		if binding == "" {
			continue
		}
		steps := strings.Split(binding, " ")
		if len(steps) > 2 || (definition.Global || definition.Typing) && len(steps) != 1 {
			return fmt.Errorf("%s: use one chord, or two for an in-app sequence", id)
		}
		if len(steps) > 1 && !strings.HasPrefix(id, "go") && id != "up" {
			return fmt.Errorf("%s: only navigation actions support two-step sequences", id)
		}
		for _, step := range steps {
			parts := strings.Split(step, "+")
			key := parts[len(parts)-1]
			if !keyPattern.MatchString(key) {
				return fmt.Errorf("%s: unsupported key %q (Escape and Tab are reserved)", id, key)
			}
			previous := -1
			modifiers := map[string]int{"Ctrl": 0, "Alt": 1, "Meta": 2, "Mod": 2, "Shift": 3}
			for _, modifier := range parts[:len(parts)-1] {
				rank, ok := modifiers[modifier]
				if !ok || rank <= previous {
					return fmt.Errorf("%s: order modifiers as Ctrl, Alt, Meta (or Mod), Shift", id)
				}
				previous = rank
			}
			if strings.Contains(step, "Mod+") && (strings.Contains(step, "Ctrl+") || strings.Contains(step, "Meta+")) {
				return fmt.Errorf("%s: Mod cannot be combined with Ctrl or Meta", id)
			}
			if definition.Typing && !strings.Contains(step, "Ctrl+") && !strings.Contains(step, "Alt+") && !strings.Contains(step, "Meta+") && !strings.Contains(step, "Mod+") {
				return fmt.Errorf("%s: use Ctrl, Alt, Meta, or Mod to avoid interfering with typing", id)
			}
			if definition.Global && (!strings.Contains(step, "Ctrl+") && !strings.Contains(step, "Alt+") && !strings.Contains(step, "Meta+") || strings.Contains(step, "Mod+")) {
				return fmt.Errorf("%s: a global shortcut needs Ctrl, Alt, or Meta", id)
			}
		}
	}

	// Expand Mod so conflicts with either platform's accelerator are detected before saving.
	bindings := map[string]string{}
	for id, definition := range definitions {
		binding := definition.Binding
		if override, ok := overrides[id]; ok {
			binding = override
		}
		if binding != "" {
			bindings[id] = binding
		}
	}
	ids := make([]string, 0, len(bindings))
	for id := range bindings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for index, id := range ids {
		for _, other := range ids[index+1:] {
			for _, left := range expandMod(bindings[id]) {
				for _, right := range expandMod(bindings[other]) {
					if left == right || strings.HasPrefix(left, right+" ") || strings.HasPrefix(right, left+" ") {
						return fmt.Errorf("%s conflicts with %s", definitions[id].Label, definitions[other].Label)
					}
				}
			}
		}
	}
	return nil
}

func expandMod(binding string) []string {
	if !strings.Contains(binding, "Mod+") {
		return []string{binding}
	}
	return []string{strings.ReplaceAll(binding, "Mod+", "Ctrl+"), strings.ReplaceAll(binding, "Mod+", "Meta+")}
}
