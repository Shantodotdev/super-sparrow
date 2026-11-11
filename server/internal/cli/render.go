package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// field is one key of a JSON object. Objects are decoded as ordered fields so the text output keeps
// the order the server sent.
type field struct {
	key   string
	value any
}

// object is a decoded JSON object.
type object []field

// renderText formats a JSON value as readable text: objects as `key: value` lines, nested values
// indented, and arrays as `- ` items.
func renderText(data []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	value, err := decodeValue(decoder)
	if err != nil {
		return "", fmt.Errorf("format response: %w", err)
	}
	return strings.Join(renderBlock(value, 0), "\n"), nil
}

// decodeValue reads the next JSON value, keeping object fields in order.
func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}

	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}

	switch delimiter {
	case '{':
		fields := object{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			fields = append(fields, field{key: key, value: value})
		}
		if _, err := decoder.Token(); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		return fields, nil
	case '[':
		items := []any{}
		for decoder.More() {
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		if _, err := decoder.Token(); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		return items, nil
	}
	return nil, fmt.Errorf("unexpected %q", delimiter)
}

// renderBlock returns the lines of value, each indented by indent spaces.
func renderBlock(value any, indent int) []string {
	pad := strings.Repeat(" ", indent)

	switch v := value.(type) {
	case object:
		if len(v) == 0 {
			return []string{pad + "{}"}
		}
		var lines []string
		for _, f := range v {
			lines = append(lines, renderField(f, indent)...)
		}
		return lines
	case []any:
		if len(v) == 0 {
			return []string{pad + "[]"}
		}
		var lines []string
		for _, item := range v {
			block := renderBlock(item, indent+2)
			// The first line of the item carries the dash in place of its own indentation.
			block[0] = pad + "- " + block[0][indent+2:]
			lines = append(lines, block...)
		}
		return lines
	}

	text := scalarText(value)
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return lines
}

// renderField returns the lines of one object field.
func renderField(f field, indent int) []string {
	pad := strings.Repeat(" ", indent)

	switch v := f.value.(type) {
	case object:
		if len(v) == 0 {
			return []string{pad + f.key + ": {}"}
		}
		return append([]string{pad + f.key + ":"}, renderBlock(v, indent+2)...)
	case []any:
		if len(v) == 0 {
			return []string{pad + f.key + ": []"}
		}
		return append([]string{pad + f.key + ":"}, renderBlock(v, indent+2)...)
	}

	text := scalarText(f.value)
	if strings.Contains(text, "\n") {
		return append([]string{pad + f.key + ":"}, renderBlock(f.value, indent+2)...)
	}
	return []string{pad + f.key + ": " + text}
}

// scalarText formats a string, number, boolean, null, or empty container.
func scalarText(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return strings.TrimRight(v, "\n")
	case json.Number:
		return v.String()
	case bool:
		return fmt.Sprint(v)
	case object:
		return "{}"
	case []any:
		return "[]"
	}
	return fmt.Sprint(value)
}
