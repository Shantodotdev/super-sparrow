package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// validateArguments enforces the advertised schema even for clients that skip input validation.
func validateArguments(args map[string]json.RawMessage, schema map[string]any) error {
	properties := schema["properties"].(map[string]any)
	for name, raw := range args {
		property, ok := properties[name]
		if !ok {
			return &store.ValidationError{Field: name, Message: "is not an argument of this tool"}
		}
		if err := validateValue(name, raw, property.(map[string]any)); err != nil {
			return err
		}
	}
	required, _ := schema["required"].([]string)
	for _, name := range required {
		if _, ok := args[name]; !ok {
			return &store.ValidationError{Field: name, Message: "is required"}
		}
	}
	return nil
}

func validateValue(field string, raw json.RawMessage, schema map[string]any) error {
	fail := func(message string) error { return &store.ValidationError{Field: field, Message: message} }
	var value any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return fail("must not be null")
	}
	switch schema["type"] {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fail("must be text")
		}
		if values, ok := schema["enum"].([]string); ok && !slices.Contains(values, text) {
			return fail(fmt.Sprintf("must be one of %v", values))
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || math.Trunc(number) != number {
			return fail("must be an integer")
		}
		if min, ok := schema["minimum"].(int); ok && number < float64(min) {
			return fail(fmt.Sprintf("must be at least %d", min))
		}
		if max, ok := schema["maximum"].(int); ok && number > float64(max) {
			return fail(fmt.Sprintf("must be at most %d", max))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fail("must be true or false")
		}
	case "array":
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return fail("must be a list")
		}
		for _, item := range items {
			if err := validateValue(field, item, schema["items"].(map[string]any)); err != nil {
				return err
			}
		}
	case "object":
		var args map[string]json.RawMessage
		if err := json.Unmarshal(raw, &args); err != nil {
			return fail("must be an object")
		}
		if err := validateArguments(args, schema); err != nil {
			return err
		}
	}
	return nil
}
