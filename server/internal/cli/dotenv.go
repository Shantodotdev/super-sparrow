package cli

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// secretNamePattern is the shape of a secret name, as the server requires.
var secretNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

// exportPrefixPattern matches the optional export keyword and the whitespace after it.
var exportPrefixPattern = regexp.MustCompile(`^export\s+`)

// maxDotEnvSize bounds how much of a .env file is read.
const maxDotEnvSize = 1 << 20

// parseDotEnv reads a .env file: NAME=value lines with an optional export prefix, single or double
// quotes, blank lines, and # comments. Double-quoted values may span lines, such as a PEM key.
// A leading byte order mark and CRLF line endings are accepted. A malformed line is an error that
// names its line number. When a name repeats, the last value wins.
func parseDotEnv(reader io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxDotEnvSize+1))
	if err != nil {
		return nil, fmt.Errorf("read .env file: %w", err)
	}
	if len(data) > maxDotEnvSize {
		return nil, fmt.Errorf(".env file is larger than %d bytes", maxDotEnvSize)
	}
	text := strings.ReplaceAll(strings.TrimPrefix(string(data), "\ufeff"), "\r\n", "\n")

	values := map[string]string{}
	number := 1
	for pos := 0; pos < len(text); {
		end := strings.IndexByte(text[pos:], '\n')
		if end < 0 {
			end = len(text) - pos
		}
		line := text[pos : pos+end]
		next := min(pos+end+1, len(text))

		work := strings.TrimLeft(line, " \t")
		offset := pos + len(line) - len(work)
		if work = strings.TrimRight(work, " \t"); work == "" || strings.HasPrefix(work, "#") {
			pos = next
			number++
			continue
		}

		if prefix := exportPrefixPattern.FindString(work); prefix != "" {
			work = work[len(prefix):]
			offset += len(prefix)
		}
		name, _, found := strings.Cut(work, "=")
		if !found {
			return nil, fmt.Errorf("line %d: expected NAME=value", number)
		}
		valueOffset := offset + len(name) + 1
		name = strings.TrimSpace(name)
		if !secretNamePattern.MatchString(name) {
			return nil, fmt.Errorf("line %d: %q is not a valid secret name (use A-Z, 0-9 and _, not starting with a digit)", number, name)
		}

		// A double-quoted value can run over several lines, so it is read from the whole text.
		rest := strings.TrimLeft(text[valueOffset:pos+end], " \t")
		if strings.HasPrefix(rest, `"`) {
			start := pos + end - len(rest) + 1
			value, closeAt, err := readDoubleQuoted(text, start)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", number, err)
			}

			lineEnd := strings.IndexByte(text[closeAt:], '\n')
			if lineEnd < 0 {
				lineEnd = len(text) - closeAt
			}
			if err := onlyComment(text[closeAt : closeAt+lineEnd]); err != nil {
				return nil, fmt.Errorf("line %d: %w", number, err)
			}
			values[name] = value
			number += strings.Count(text[pos:closeAt], "\n") + 1
			pos = min(closeAt+lineEnd+1, len(text))
			continue
		}

		value, err := parseDotEnvValue(strings.TrimSpace(rest))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", number, err)
		}
		values[name] = value
		pos = next
		number++
	}
	return values, nil
}

// readDoubleQuoted reads a double-quoted value that starts at text[start] (just after the opening
// quote) and returns it with the index just after the closing quote. Only \n, \r, \t, \\ and \"
// are unescaped; any other backslash stays as it is.
func readDoubleQuoted(text string, start int) (string, int, error) {
	var value strings.Builder
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '"':
			return value.String(), i + 1, nil
		case '\\':
			if i+1 == len(text) {
				return "", 0, errors.New("double quote is not closed")
			}
			i++
			switch text[i] {
			case 'n':
				value.WriteByte('\n')
			case 'r':
				value.WriteByte('\r')
			case 't':
				value.WriteByte('\t')
			case '"', '\\':
				value.WriteByte(text[i])
			default:
				value.WriteByte('\\')
				value.WriteByte(text[i])
			}
		default:
			value.WriteByte(text[i])
		}
	}
	return "", 0, errors.New("double quote is not closed")
}

// parseDotEnvValue reads the right-hand side of NAME=value on one line, which is single-quoted or
// unquoted; double-quoted values are read by readDoubleQuoted.
func parseDotEnvValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	if raw[0] == '\'' {
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", errors.New("single quote is not closed")
		}
		if err := onlyComment(raw[end+2:]); err != nil {
			return "", err
		}
		return raw[1 : end+1], nil
	}

	// An unquoted value ends at a # that follows a space.
	if index := strings.Index(raw, " #"); index >= 0 {
		raw = raw[:index]
	}
	return strings.TrimSpace(raw), nil
}

// onlyComment checks that nothing but whitespace or a comment follows a closing quote.
func onlyComment(rest string) error {
	rest = strings.TrimSpace(rest)
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return errors.New("unexpected text after the closing quote")
	}
	return nil
}
