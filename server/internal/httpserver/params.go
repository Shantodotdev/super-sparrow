package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// projectFromPath loads the project named by the {key} path segment, such as WP.
func (s *server) projectFromPath(r *http.Request) (store.Project, error) {
	return s.store.GetProjectByKey(r.Context(), r.PathValue("key"))
}

// issueFromPath loads the issue named by the {key} path segment, such as WP-142.
func (s *server) issueFromPath(r *http.Request) (store.Issue, error) {
	return s.store.GetIssueByKey(r.Context(), r.PathValue("key"))
}

// roundFromPath loads the issue and its review round named by the {key} and {round} segments.
func (s *server) roundFromPath(r *http.Request) (store.Issue, store.ReviewRound, error) {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return store.Issue{}, store.ReviewRound{}, err
	}

	number, err := parsePositive("round", r.PathValue("round"))
	if err != nil {
		return store.Issue{}, store.ReviewRound{}, err
	}

	round, err := s.store.GetReviewRoundByNumber(r.Context(), issue.ID, number)
	return issue, round, err
}

// findingFromPath loads the issue and the finding named by the {key}, {round}, and {finding}
// segments, as in /api/issues/WP-142/reviews/2/findings/R3.
func (s *server) findingFromPath(r *http.Request) (store.Issue, store.Finding, error) {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return store.Issue{}, store.Finding{}, err
	}

	round, err := parsePositive("round", r.PathValue("round"))
	if err != nil {
		return store.Issue{}, store.Finding{}, err
	}
	number, err := store.ParseFindingLabel("finding", r.PathValue("finding"))
	if err != nil {
		return store.Issue{}, store.Finding{}, err
	}

	finding, err := s.store.GetFindingByNumber(r.Context(), issue.ID, round, number)
	return issue, finding, err
}

// parsePositive reads a whole number of at least 1 written plainly, without signs, spaces, or
// leading zeros, so each value has one spelling.
func parsePositive(field, value string) (int, error) {
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 || strconv.Itoa(number) != value {
		return 0, &store.ValidationError{Field: field, Message: "must be a positive whole number"}
	}
	return number, nil
}

// queryInt reads a whole number from the query string, or 0 when it is absent.
func queryInt(r *http.Request, name string) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		return 0, &store.ValidationError{Field: name, Message: "must be a whole number"}
	}
	return number, nil
}

// queryLimit reads ?limit, the page size, or 0 when it is absent so the default applies.
func queryLimit(r *http.Request) (int, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return 0, nil
	}

	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > store.MaximumPageLimit {
		return 0, &store.ValidationError{Field: "limit", Message: fmt.Sprintf("must be a whole number from 1 to %d", store.MaximumPageLimit)}
	}
	return limit, nil
}

// queryList reads a comma-separated list from the query string, without empty entries.
func queryList(r *http.Request, name string) []string {
	var values []string
	for value := range strings.SplitSeq(r.URL.Query().Get(name), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// queryBool reads true or false from the query string, or false when it is absent.
func queryBool(r *http.Request, name string) (bool, error) {
	switch r.URL.Query().Get(name) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	}
	return false, &store.ValidationError{Field: name, Message: "must be true or false"}
}

// renameField reports a validation error about a store field under the name the request used
// for it, such as ?status for the store's list of statuses.
func renameField(err error, from, to string) error {
	if validation, ok := errors.AsType[*store.ValidationError](err); ok && validation.Field == from {
		return &store.ValidationError{Field: to, Message: validation.Message}
	}
	return err
}

// patch turns a field of an update body into a store update: absent leaves the field as it is,
// and a value sets it. null reads as the empty value, so null and "" both clear an optional
// field and a required field refuses both.
func patch[T any](value *T, null bool) store.Field[T] {
	switch {
	case value != nil:
		return store.Change(*value)
	case null:
		var empty T
		return store.Change(empty)
	}
	return store.Field[T]{}
}

// pageSize returns the number of records a full page holds for a requested limit.
func pageSize(limit int) int {
	if limit == 0 {
		return store.DefaultPageLimit
	}
	return limit
}
