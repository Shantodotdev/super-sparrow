package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// apiHandler handles one request. A returned error becomes the JSON error response.
type apiHandler func(w http.ResponseWriter, r *http.Request) error

// errorBody is every error response. Field names the request field at fault, when there is one.
type errorBody struct {
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError maps err to a status and a message. Unexpected errors are logged and answered with
// a generic message, so internal details never reach the client.
func (s *server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *store.ValidationError
	var limited *auth.RateLimitError

	switch {
	case errors.As(err, &validation):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: validation.Message, Field: validation.Field})
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
		writeJSON(w, http.StatusTooManyRequests, errorBody{Error: limited.Error()})
	case errors.Is(err, store.ErrInvalidCode):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: store.ErrInvalidCode.Error(), Field: "code"})
	case errors.Is(err, store.ErrUnauthenticated):
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: "sign in or send a valid agent token"})
	case errors.Is(err, store.ErrForbidden):
		writeJSON(w, http.StatusForbidden, errorBody{Error: store.Explain(err)})
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: store.Explain(err)})
	case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrInvalidTransition):
		writeJSON(w, http.StatusConflict, errorBody{Error: store.Explain(err)})
	default:
		s.logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal server error"})
	}
}

// decodeUpdate reads an update body like decodeJSON and also returns the names of the fields
// sent as null, which the pointer fields of dst cannot tell from absent ones.
func decodeUpdate(w http.ResponseWriter, r *http.Request, dst any) (map[string]bool, error) {
	var body bytes.Buffer
	r.Body = io.NopCloser(io.TeeReader(r.Body, &body))
	if err := decodeJSON(w, r, dst); err != nil {
		return nil, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body.Bytes(), &fields); err != nil {
		return nil, &store.ValidationError{Field: "body", Message: "must be a JSON object"}
	}
	nulls := make(map[string]bool)
	for name, value := range fields {
		if string(value) == "null" {
			nulls[name] = true
		}
	}
	return nulls, nil
}

// decodeJSON reads the request body into dst, refusing unknown fields, trailing data, and
// bodies over maxBodyBytes. Its errors name the field at fault where JSON allows.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()

	err := decoder.Decode(dst)
	var typeErr *json.UnmarshalTypeError
	var tooLarge *http.MaxBytesError
	switch {
	case err == nil:
	case errors.Is(err, io.EOF):
		return &store.ValidationError{Field: "body", Message: "is required"}
	case errors.As(err, &typeErr):
		return store.TypeMismatch(typeErr.Field, typeErr.Type)
	case errors.As(err, &tooLarge):
		return &store.ValidationError{Field: "body", Message: fmt.Sprintf("must be at most %d bytes", maxBodyBytes)}
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return &store.ValidationError{Field: field, Message: "is not a known field"}
	default:
		return &store.ValidationError{Field: "body", Message: "must be valid JSON"}
	}

	if decoder.More() {
		return &store.ValidationError{Field: "body", Message: "must hold one JSON object"}
	}
	return nil
}
