package store

import (
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxNameLength     = 100
	maxTitleLength    = 200
	maxLabelLength    = 50
	maxTextLength     = 100_000
	maxCommentLength  = 20_000
	maxPathLength     = 1_000
	maxBranchLength   = 250
	maxURLLength      = 2_000
	maxEmailLength    = 254
	maxPlanCoverage   = 2_000
	maxReportedModel  = 100
	defaultPageLimit  = 50
	maximumPageLimit  = 200
	maxLabelsPerIssue = 20
)

var (
	projectKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)
	agentNamePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

// requiredText trims value and checks that it is present and at most max characters long.
func requiredText(field, value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid(field, "is required")
	}

	return value, checkLength(field, value, max)
}

// singleLineText is requiredText for values shown on one line, such as names and labels, which
// may also reach email subjects. It refuses line breaks and other control characters.
func singleLineText(field, value string, max int) (string, error) {
	value, err := requiredText(field, value, max)
	if err != nil {
		return "", err
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", invalid(field, "must be one line without control characters")
	}
	return value, nil
}

// optionalText trims value and checks that it is at most max characters long. It may be empty.
func optionalText(field, value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	return value, checkLength(field, value, max)
}

func checkLength(field, value string, max int) error {
	if !utf8.ValidString(value) {
		return invalid(field, "must be valid UTF-8")
	}
	if utf8.RuneCountInString(value) > max {
		return invalid(field, "must be at most %d characters", max)
	}
	return nil
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if email == "" {
		return "", invalid("email", "is required")
	}
	if len(email) > maxEmailLength {
		return "", invalid("email", "must be at most %d characters", maxEmailLength)
	}

	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", invalid("email", "must be a plain email address such as name@example.com")
	}

	return email, nil
}

func optionalCommit(field, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "" && !commitPattern.MatchString(value) {
		return "", invalid(field, "must be a commit SHA of 7 to 64 hex characters")
	}
	return value, nil
}

func requiredCommit(field, value string) (string, error) {
	commit, err := optionalCommit(field, value)
	if err != nil {
		return "", err
	}
	if commit == "" {
		return "", invalid(field, "is required")
	}
	return commit, nil
}

func optionalBranch(value string) (string, error) {
	branch := strings.TrimSpace(value)
	if branch == "" {
		return "", nil
	}
	if len(branch) > maxBranchLength || strings.ContainsAny(branch, " ~^:?*[\\") ||
		strings.Contains(branch, "..") || strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") {
		return "", invalid("branch_name", "must be a valid git branch name")
	}
	return branch, nil
}

func optionalHTTPURL(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maxURLLength {
		return "", invalid(field, "must be at most %d characters", maxURLLength)
	}

	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", invalid(field, "must be an http or https URL")
	}
	return value, nil
}

// pageLimit returns limit when it is within range, the default page size when it is zero, and
// an error otherwise.
func pageLimit(limit int) (int, error) {
	switch {
	case limit == 0:
		return defaultPageLimit, nil
	case limit < 0 || limit > maximumPageLimit:
		return 0, invalid("limit", "must be between 1 and %d", maximumPageLimit)
	}
	return limit, nil
}
