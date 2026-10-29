package linear

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// A review comment looks like this; the parser accepts small variations of each part, such as
// bold labels or headings, and fields in any order:
//
//	## Review round 2
//
//	Reviewed: a1b2c3d..e4f5a6b (includes uncommitted changes)
//	Plan coverage: Steps 1 to 3.
//
//	### R1 [Blocking] Text can arrive before its file
//	**Problem:** The worker sends whichever row it claims first.
//	**Fix:** Send both from one job.
//	**Evidence:** `apps/api/src/staff-reply.ts:189`
//	**Status:** fixed
//
//	### R2 (Nit) Rename the helper
//	...
//
//	### Summary
//	Two findings; one blocks the merge.

// missingFix stands in for a finding whose review gave no fix, since every finding needs one.
const missingFix = "Not stated in the imported review."

var (
	roundHeading   = regexp.MustCompile(`(?i)^#{1,3}\s*review round\s+(\d{1,4})\b`)
	findingHeading = regexp.MustCompile(`(?i)^(?:#{2,5}\s*\**|\*\*)\s*R(\d{1,4})\b\**[\s:.\-–—]*(.*?)\**\s*$`)
	otherHeading   = regexp.MustCompile(`^#{1,5}\s+(.*)$`)
	fieldLine      = regexp.MustCompile(`^(?:[-*]\s+)?\**([A-Za-z][A-Za-z '’]{1,30}?)\**\s*:\**\s*(.*)$`)
	commitRange    = regexp.MustCompile(`(?i)\b([0-9a-f]{7,40})\b\s*(?:\.\.\.?|->|→|to)\s*\b([0-9a-f]{7,40})\b`)
	singleCommit   = regexp.MustCompile(`(?i)\b([0-9a-f]{7,40})\b`)
	fileAndLine    = regexp.MustCompile("`?([A-Za-z0-9_./@-]+\\.[A-Za-z0-9]+)(?::(\\d+))?(?:[-–]\\d+)?`?")
	severityMarker = regexp.MustCompile(`(?i)^(?:\[([^\]]+)\]|\(([^)]+)\)|\*\*([^*]+)\*\*|([a-z][a-z _-]{1,12}):)\s*`)
)

// ParsedRound is a review round read from a comment.
type ParsedRound struct {
	Number              int
	FromCommit          string
	ToCommit            string
	IncludesUncommitted bool
	PlanCoverage        string
	Summary             string
	Findings            []store.ImportedFinding
}

// ParseReviewRound reads a review round from a comment's markdown. It returns false when the
// comment has no "Review round N" heading.
func ParseReviewRound(body string) (ParsedRound, bool) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")

	start := -1
	var round ParsedRound
	for index, line := range lines {
		if match := roundHeading.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			round.Number, _ = strconv.Atoi(match[1])
			start = index + 1
			break
		}
	}
	if start < 0 {
		return ParsedRound{}, false
	}

	var finding *findingDraft
	var drafts []*findingDraft
	section := "preamble"
	var summary []string

	for _, raw := range lines[start:] {
		line := strings.TrimSpace(raw)

		if match := findingHeading.FindStringSubmatch(line); match != nil {
			number, _ := strconv.Atoi(match[1])
			finding = newFindingDraft(number, match[2])
			drafts = append(drafts, finding)
			section = "finding"
			continue
		}
		if match := otherHeading.FindStringSubmatch(line); match != nil {
			heading := strings.ToLower(strings.Trim(match[1], "* "))
			if strings.HasPrefix(heading, "summary") || strings.HasPrefix(heading, "verdict") || strings.HasPrefix(heading, "overall") {
				section, finding = "summary", nil
				continue
			}
		}

		switch section {
		case "preamble":
			round.readPreamble(line)
		case "summary":
			summary = append(summary, raw)
		case "finding":
			finding.read(line, raw)
		}
	}

	if round.Summary == "" {
		round.Summary = strings.TrimSpace(strings.Join(summary, "\n"))
	}
	for _, draft := range drafts {
		round.Findings = append(round.Findings, draft.finish())
	}
	return round, true
}

// readPreamble reads the round's details from the lines before its first finding.
func (round *ParsedRound) readPreamble(line string) {
	if line == "" {
		return
	}

	if lower := strings.ToLower(line); strings.Contains(lower, "uncommitted") {
		round.IncludesUncommitted = !excludesUncommitted(lower)
	}

	match := fieldLine.FindStringSubmatch(line)
	if match == nil {
		return
	}
	key, value := strings.ToLower(strings.TrimSpace(match[1])), strings.TrimSpace(match[2])

	switch {
	case strings.Contains(key, "coverage"):
		round.PlanCoverage = value
	case key == "summary" || key == "verdict":
		round.Summary = value
	case strings.Contains(key, "review") || strings.Contains(key, "range") || strings.Contains(key, "commit"):
		if commits := commitRange.FindStringSubmatch(value); commits != nil {
			round.FromCommit, round.ToCommit = strings.ToLower(commits[1]), strings.ToLower(commits[2])
		} else if commit := singleCommit.FindStringSubmatch(value); commit != nil && round.ToCommit == "" {
			round.ToCommit = strings.ToLower(commit[1])
		}
	}
}

// excludesUncommitted reports whether a line about uncommitted changes says they were left out,
// as in "without uncommitted changes" or "Uncommitted changes: no".
func excludesUncommitted(lower string) bool {
	for _, phrase := range []string{"without", "excluding", "excluded", "not included"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return strings.HasSuffix(strings.TrimRight(lower, ".*) "), ": no")
}

// findingDraft collects one finding's fields as its lines are read.
type findingDraft struct {
	finding store.ImportedFinding
	fields  map[string]*strings.Builder
	current string
}

func newFindingDraft(number int, heading string) *findingDraft {
	draft := &findingDraft{
		finding: store.ImportedFinding{Number: number, Status: store.FindingOpen},
		fields:  map[string]*strings.Builder{}, current: "problem",
	}

	// The heading may lead with a severity, as in "[Blocking] Title" or "Nit: Title".
	title := strings.TrimSpace(heading)
	if match := severityMarker.FindStringSubmatch(title); match != nil {
		marker := strings.Join(match[1:], "")
		if severity, ok := parseSeverity(marker); ok {
			draft.finding.Severity = severity
			title = strings.TrimSpace(title[len(match[0]):])
		}
	}
	draft.finding.Title = strings.Trim(title, "*_ ")
	return draft
}

// read adds one line of the finding. A field label starts a field; other lines continue the
// current one.
func (d *findingDraft) read(line, raw string) {
	if match := fieldLine.FindStringSubmatch(line); match != nil {
		if field, ok := fieldName(match[1]); ok {
			d.current = field
			d.append(field, match[2])
			return
		}
	}
	d.append(d.current, raw)
}

func (d *findingDraft) append(field, text string) {
	builder, ok := d.fields[field]
	if !ok {
		builder = &strings.Builder{}
		d.fields[field] = builder
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(text)
}

func (d *findingDraft) field(name string) string {
	if builder, ok := d.fields[name]; ok {
		return strings.TrimSpace(builder.String())
	}
	return ""
}

func (d *findingDraft) finish() store.ImportedFinding {
	f := d.finding
	f.Problem, f.Fix, f.Evidence = d.field("problem"), d.field("fix"), d.field("evidence")
	f.DecisionReason = d.field("reason")

	if severity, ok := parseSeverity(d.field("severity")); ok {
		f.Severity = severity
	}
	if f.Severity == "" {
		f.Severity = store.SeverityShouldFix
	}
	if status, ok := parseStatus(d.field("status")); ok {
		f.Status = status
	}
	if f.Title == "" {
		f.Title = firstLine(f.Problem)
	}
	if f.Title == "" {
		f.Title = "Finding R" + strconv.Itoa(f.Number)
	}
	if f.Problem == "" {
		f.Problem = f.Title
	}
	if f.Fix == "" {
		f.Fix = missingFix
	}

	// The location is its own field, or else a path with a folder in the evidence.
	if match := fileAndLine.FindStringSubmatch(d.field("file")); match != nil {
		f.FilePath = match[1]
		f.Line, _ = strconv.Atoi(match[2])
	} else if match := fileAndLine.FindStringSubmatch(f.Evidence); match != nil && strings.Contains(match[1], "/") {
		f.FilePath = match[1]
		f.Line, _ = strconv.Atoi(match[2])
	}
	if commit := singleCommit.FindStringSubmatch(d.field("commit")); commit != nil {
		f.CommitSHA = strings.ToLower(commit[1])
	}
	return f
}

// fieldName maps a finding's field label to the field it fills.
func fieldName(label string) (string, bool) {
	label = strings.ToLower(strings.TrimSpace(label))
	switch label {
	case "problem", "issue", "what", "description", "details":
		return "problem", true
	case "fix", "suggested fix", "suggestion", "recommendation", "proposed fix", "remedy":
		return "fix", true
	case "evidence", "proof", "repro", "reproduction":
		return "evidence", true
	case "file", "location", "where", "path":
		return "file", true
	case "commit":
		return "commit", true
	case "severity", "priority":
		return "severity", true
	case "status", "decision", "state":
		return "status", true
	case "reason", "decision reason", "why", "rationale":
		return "reason", true
	}
	return "", false
}

func parseSeverity(value string) (store.Severity, bool) {
	value = strings.ToLower(strings.Trim(value, "*_[]() "))
	switch {
	case value == "":
		return "", false
	case strings.Contains(value, "block"), strings.Contains(value, "critical"), value == "high", value == "p0", value == "p1":
		return store.SeverityBlocking, true
	case strings.Contains(value, "should"), value == "major", value == "medium", value == "p2":
		return store.SeverityShouldFix, true
	case strings.HasPrefix(value, "nit"), value == "minor", value == "low", value == "p3":
		return store.SeverityNit, true
	}
	return "", false
}

func parseStatus(value string) (store.FindingStatus, bool) {
	value = strings.ToLower(strings.Trim(value, "*_.✅ "))
	switch {
	case value == "":
		return "", false
	case strings.HasPrefix(value, "verified"):
		return store.FindingVerified, true
	case strings.HasPrefix(value, "fixed"), strings.HasPrefix(value, "done"), strings.HasPrefix(value, "resolved"):
		return store.FindingFixed, true
	case strings.HasPrefix(value, "wontfix"), strings.HasPrefix(value, "won't fix"), strings.HasPrefix(value, "won’t fix"),
		strings.HasPrefix(value, "will not fix"), strings.HasPrefix(value, "rejected"):
		return store.FindingWontfix, true
	case strings.HasPrefix(value, "later"), strings.HasPrefix(value, "deferred"), strings.HasPrefix(value, "follow"):
		return store.FindingLater, true
	case strings.HasPrefix(value, "fix"), strings.HasPrefix(value, "accepted"), strings.HasPrefix(value, "to do"), strings.HasPrefix(value, "todo"):
		return store.FindingFix, true
	case strings.HasPrefix(value, "open"):
		return store.FindingOpen, true
	}
	return "", false
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if runes := []rune(line); len(runes) > 200 {
		line = string(runes[:200])
	}
	return strings.TrimSpace(line)
}
