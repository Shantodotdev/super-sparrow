package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// fakeLinear answers the importer's queries with one project, a milestone, three issues, and
// comments on the first issue.
func fakeLinear(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "lin_api_test" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":[{"message":"Authentication required"}]}`))
			return
		}

		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode query: %v", err)
		}

		var data string
		switch {
		case strings.Contains(request.Query, "projects("):
			data = `{"projects":{"nodes":[{"id":"p1","name":"Version 01","teams":{"nodes":[{"key":"WP"}]}},
				{"id":"p2","name":"Other","teams":{"nodes":[{"key":"OT"}]}}],"pageInfo":{"hasNextPage":false}}}`
		case strings.Contains(request.Query, "projectMilestones"):
			data = `{"project":{"projectMilestones":{"nodes":[{"id":"m1","name":"Phase 1","sortOrder":1}],"pageInfo":{"hasNextPage":false}}}}`
		case strings.Contains(request.Query, "issues("):
			data = `{"project":{"issues":{"nodes":[
				{"id":"i142","identifier":"WP-142","number":142,"title":"Meta comments","description":"- [x] Plan",
				 "branchName":"feat/wp-142-meta","completedAt":"2026-01-05T10:00:00Z","state":{"type":"completed","name":"Done"},
				 "team":{"key":"WP"},"projectMilestone":{"id":"m1"},"assignee":{"email":"olive@example.com"},
				 "labels":{"nodes":[{"name":"backend"}]}},
				{"id":"i143","identifier":"WP-143","number":143,"title":"Child","description":"","state":{"type":"started","name":"In Review"},
				 "team":{"key":"WP"},"parent":{"id":"i142"},"labels":{"nodes":[]}},
				{"id":"o1","identifier":"OT-1","number":1,"title":"Moved","description":"","state":{"type":"backlog","name":"Backlog"},
				 "team":{"key":"OT"},"labels":{"nodes":[]}}
			],"pageInfo":{"hasNextPage":false}}}}`
		case strings.Contains(request.Query, "comments("):
			if request.Variables["id"] != "i142" {
				data = `{"issue":{"comments":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`
				break
			}
			review, _ := json.Marshal(reviewComment)
			data = `{"issue":{"comments":{"nodes":[
				{"id":"c2","body":` + string(review) + `,"createdAt":"2026-01-03T09:00:00Z","user":{"name":"Codex"}},
				{"id":"c1","body":"Starting now.","createdAt":"2026-01-02T09:00:00Z","user":{"name":"Shanto"}},
				{"id":"c3","body":"## Review round 3\nNo commit here.","createdAt":"2026-01-04T09:00:00Z","user":null}
			],"pageInfo":{"hasNextPage":false}}}}`
		default:
			t.Errorf("unexpected query %s", request.Query)
		}
		_, _ = w.Write([]byte(`{"data":` + data + `}`))
	}))
}

func TestImportBringsInTheProjectOnce(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, time.Now)
	owner, err := st.CreateFirstOwner(ctx, "Olive Owner", "olive@example.com")
	if err != nil {
		t.Fatal(err)
	}

	server := fakeLinear(t)
	defer server.Close()
	client := NewClient(server.URL, "lin_api_test")

	report, err := Import(ctx, st, client, "version 01", "")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.ProjectKey != "WP" || report.Milestones != 1 || report.Issues != 2 || report.Comments != 3 ||
		report.ReviewRounds != 1 || report.Findings != 3 || len(report.Warnings) != 2 {
		t.Fatalf("report %+v", report)
	}

	issue, err := st.GetIssueByKey(ctx, "WP-142")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != store.IssueDone || issue.AssigneeActorID != owner.ID || len(issue.LabelIDs) != 1 || issue.MilestoneID == "" ||
		issue.CompletedAt == nil || !issue.CompletedAt.Equal(time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("WP-142 %+v", issue)
	}
	child, err := st.GetIssueByKey(ctx, "WP-143")
	if err != nil || child.ParentIssueID != issue.ID || child.Status != store.IssueInReview {
		t.Fatalf("WP-143 %+v (%v)", child, err)
	}

	comments, err := st.ListComments(ctx, issue.ID, "")
	if err != nil || len(comments) != 3 || !strings.HasPrefix(comments[0].Body, "*Imported from Linear. Written by Shanto on 2026-01-02 09:00:00 UTC.*") {
		t.Fatalf("comments %+v (%v)", comments, err)
	}
	round, err := st.GetReviewRoundByNumber(ctx, issue.ID, 2)
	if err != nil || round.ToCommit != "e4f5a6b" || round.AuthorActorID != store.SystemActorID {
		t.Fatalf("round %+v (%v)", round, err)
	}

	milestones, err := st.ListMilestones(ctx, issue.ProjectID)
	if err != nil || len(milestones) != 1 || milestones[0].Status != store.MilestoneDone {
		t.Fatalf("milestones %+v (%v)", milestones, err)
	}

	again, err := Import(ctx, st, client, "p1", "")
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if after, _ := st.ListComments(ctx, issue.ID, ""); len(after) != 3 {
		t.Fatalf("a second import added comments: %d", len(after))
	}
	if rounds, _ := st.ListReviewRounds(ctx, issue.ID); len(rounds) != 1 {
		t.Fatalf("a second import added rounds: %d", len(rounds))
	}
	if again.Issues != 2 {
		t.Errorf("second report %+v", again)
	}
}

func TestImportReportsLinearErrors(t *testing.T) {
	server := fakeLinear(t)
	defer server.Close()

	_, err := NewClient(server.URL, "wrong").FindProject(context.Background(), "Version 01")
	if err == nil || !strings.Contains(err.Error(), "Authentication required") {
		t.Fatalf("error = %v", err)
	}
	if _, err := NewClient(server.URL, "lin_api_test").FindProject(context.Background(), "Missing"); err == nil {
		t.Fatal("found a missing project")
	}
}
