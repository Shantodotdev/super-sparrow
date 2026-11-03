package httpserver

import (
	"net/http"
	"slices"
	"testing"
)

func TestOwnerActionsNeedABrowser(t *testing.T) {
	s := newSecretsTeam(t)

	// An owner's CLI session, or an agent holding it, cannot add people, grant the staging and
	// production permission, or delete projects, any of which would reach what only a browser reads.
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/people", map[string]string{"display_name": "Mallory", "email": "mallory@example.com", "role": "owner"}},
		{http.MethodPatch, "/api/people/" + s.memberID + "/environments", map[string]bool{"can_manage_environments": true}},
		{http.MethodPost, "/api/people/" + s.memberID + "/disable", nil},
		{http.MethodDelete, "/api/projects/WP", nil},
		{http.MethodPost, "/api/projects/WP/secrets/enable", nil},
	} {
		wantStatus(t, s.request(call.method, call.path, s.ownerCLI, call.body), http.StatusForbidden)
	}
	wantStatus(t, s.request(http.MethodPatch, "/api/people/"+s.memberID+"/environments", s.owner, map[string]bool{"can_manage_environments": true}), http.StatusOK)
}

func TestListSecretNamesShowsNamesButNoValues(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("staging", s.owner, nil, map[string]string{"DEPLOY_HOST": "staging.example.com"}, nil, http.StatusOK)
	s.patch("dev-defaults", s.owner, nil, map[string]string{"SHARED": "team"}, nil, http.StatusOK)
	s.patch("dev", s.memberCLI, nil, map[string]string{"MINE": "secret-value"}, nil, http.StatusOK)
	s.call(http.MethodPut, "/api/projects/WP/secrets/production/required", s.owner, map[string][]string{"names": {"DEPLOY_HOST"}}, http.StatusOK, nil)

	// The agent token belongs to the member, so the agent sees the member's dev.
	output := s.toolObject("list_secret_names", map[string]any{"project_key": "WP"})
	if output["enabled"] != true || !slices.Contains(output["staging"].([]any), any("DEPLOY_HOST")) {
		t.Fatalf("list_secret_names = %+v", output)
	}
	dev := map[string]string{}
	for _, entry := range output["dev"].([]any) {
		item := entry.(map[string]any)
		dev[item["name"].(string)] = item["source"].(string)
	}
	if dev["MINE"] != "override" || dev["SHARED"] != "default" {
		t.Fatalf("dev = %v", dev)
	}
	missing := output["missing"].(map[string]any)["production"].([]any)
	if len(missing) != 1 || missing[0] != "DEPLOY_HOST" {
		t.Fatalf("missing production = %v", missing)
	}
}
