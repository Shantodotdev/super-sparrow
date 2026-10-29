package linear

import (
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const reviewComment = `## Review round 2

Reviewed: A1B2C3D..e4f5a6b (includes uncommitted changes)
**Plan coverage:** Steps 1 to 3.

### R1 [Blocking] Text can arrive before its file
**Problem:** The worker sends whichever row it claims first.
It can send the text before the file.
**Fix:** Send both from one job.
**Evidence:** ` + "`apps/api/src/staff-reply.ts:189`" + ` shows the claim.
**Status:** fixed

#### R2 (Nit) Rename the helper
- **Suggestion**: Call it sendReply.
- **Status**: won't fix
- **Reason**: The name matches the API.

**R3 Should fix: Missing test**
The new branch has no test.

### Summary
Three findings; one blocks the merge.
`

func TestParseReviewRound(t *testing.T) {
	round, ok := ParseReviewRound("Some intro.\n\n" + reviewComment)
	if !ok {
		t.Fatal("no round found")
	}

	if round.Number != 2 || round.FromCommit != "a1b2c3d" || round.ToCommit != "e4f5a6b" || !round.IncludesUncommitted {
		t.Errorf("round %+v", round)
	}
	if round.PlanCoverage != "Steps 1 to 3." || round.Summary != "Three findings; one blocks the merge." {
		t.Errorf("coverage %q, summary %q", round.PlanCoverage, round.Summary)
	}
	if len(round.Findings) != 3 {
		t.Fatalf("findings %+v", round.Findings)
	}

	first := round.Findings[0]
	if first.Number != 1 || first.Severity != store.SeverityBlocking || first.Title != "Text can arrive before its file" ||
		first.Problem != "The worker sends whichever row it claims first.\nIt can send the text before the file." ||
		first.Fix != "Send both from one job." || first.Status != store.FindingFixed ||
		first.FilePath != "apps/api/src/staff-reply.ts" || first.Line != 189 {
		t.Errorf("R1 %+v", first)
	}

	second := round.Findings[1]
	if second.Severity != store.SeverityNit || second.Title != "Rename the helper" || second.Fix != "Call it sendReply." ||
		second.Status != store.FindingWontfix || second.DecisionReason != "The name matches the API." || second.Problem != "Rename the helper" {
		t.Errorf("R2 %+v", second)
	}

	third := round.Findings[2]
	if third.Number != 3 || third.Severity != store.SeverityShouldFix || third.Title != "Missing test" ||
		third.Problem != "The new branch has no test." || third.Fix != missingFix || third.Status != store.FindingOpen {
		t.Errorf("R3 %+v", third)
	}
}

func TestParseReviewRoundIgnoresOtherComments(t *testing.T) {
	if _, ok := ParseReviewRound("Looks good to me. R1 is fine."); ok {
		t.Error("found a round in a plain comment")
	}
}

func TestParseReviewRoundWithASingleCommit(t *testing.T) {
	round, ok := ParseReviewRound("# Review round 1\nCommit: 0123abcd, without uncommitted changes\n\n## R1: blocking: Leak\nProblem: x")
	if !ok || round.ToCommit != "0123abcd" || round.FromCommit != "" || round.IncludesUncommitted {
		t.Fatalf("round %+v", round)
	}
	if len(round.Findings) != 1 || round.Findings[0].Severity != store.SeverityBlocking || round.Findings[0].Title != "Leak" {
		t.Fatalf("findings %+v", round.Findings)
	}
}
