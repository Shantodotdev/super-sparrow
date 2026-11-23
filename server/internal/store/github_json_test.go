package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGitHubJSONWritesMissingValuesAsNull(t *testing.T) {
	repository, err := json.Marshal(GitHubRepository{ID: 1, FullName: "o/r"})
	if err != nil || !strings.Contains(string(repository), `"project_key":null`) || strings.Count(string(repository), "project_key") != 1 {
		t.Fatalf("repository JSON = %s, %v", repository, err)
	}
	linked, _ := json.Marshal(GitHubRepository{ID: 1, ProjectKey: "SS"})
	if !strings.Contains(string(linked), `"project_key":"SS"`) {
		t.Fatalf("linked repository JSON = %s", linked)
	}

	pull, err := json.Marshal(GitHubPullRequest{ID: 2, IssueKey: "SS-1"})
	if err != nil || !strings.Contains(string(pull), `"author_person_id":null`) || !strings.Contains(string(pull), `"issue_key":"SS-1"`) ||
		strings.Count(string(pull), "issue_key") != 1 {
		t.Fatalf("pull request JSON = %s, %v", pull, err)
	}
}
