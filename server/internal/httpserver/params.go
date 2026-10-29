package httpserver

import (
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

func parsePositive(field, value string) (int, error) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number < 1 {
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

// optionalField turns an optional JSON value into a store update: absent leaves the field as it
// is, and a value, even empty, sets it.
func optionalField[T any](value *T) store.Field[T] {
	if value == nil {
		return store.Field[T]{}
	}
	return store.Change(*value)
}

// pageSize returns the number of records a full page holds for a requested limit.
func pageSize(limit int) int {
	if limit == 0 {
		return store.DefaultPageLimit
	}
	return limit
}
