package httpserver

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func TestSubissues(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()
	path := "/api/issues/WP-1/subissues"
	var list struct {
		Subissues []subissueView `json:"subissues"`
	}
	a.call(http.MethodGet, path, a.member, nil, http.StatusOK, &list)
	if list.Subissues == nil || len(list.Subissues) != 0 {
		t.Fatalf("empty list: %+v", list)
	}
	a.call(http.MethodGet, path, "", nil, http.StatusUnauthorized, nil)
	a.call(http.MethodPost, path, "", map[string]string{"title": "Denied", "status": "todo"}, http.StatusUnauthorized, nil)

	var created struct {
		Subissue subissueView `json:"subissue"`
	}
	a.call(http.MethodPost, path, a.agent, map[string]string{
		"title": "First child", "description": "## Task\n\n**Markdown**", "status": "todo",
	}, http.StatusCreated, &created)
	child := created.Subissue
	if child.Key != "WP-1.1" || child.IssueID != issue.ID || child.DescriptionRevision != 1 || child.Description != "## Task\n\n**Markdown**" {
		t.Fatalf("created: %+v", child)
	}
	a.call(http.MethodPost, path, a.member, map[string]string{"title": "Second child", "status": "backlog"}, http.StatusCreated, &created)
	if created.Subissue.Key != "WP-1.2" {
		t.Fatalf("second: %+v", created)
	}
	editPath := path + "/" + child.ID
	a.call(http.MethodPatch, editPath, a.member, map[string]any{
		"title": "Edited child", "description": "New **description**", "description_revision": 1, "status": "in_progress",
	}, http.StatusOK, &created)
	if created.Subissue.Title != "Edited child" || created.Subissue.DescriptionRevision != 2 || created.Subissue.Status != "in_progress" {
		t.Fatalf("edited: %+v", created)
	}
	a.call(http.MethodPatch, editPath, a.agent, map[string]any{
		"description": "Stale description", "description_revision": 1, "title": "Must roll back",
	}, http.StatusConflict, nil)
	a.call(http.MethodPatch, editPath, a.agent, map[string]string{"status": "done"}, http.StatusOK, nil)
	a.call(http.MethodGet, path, a.agent, nil, http.StatusOK, &list)
	if len(list.Subissues) != 2 || list.Subissues[0].Title != "Edited child" || list.Subissues[0].Description != "New **description**" || list.Subissues[0].Status != "done" {
		t.Fatalf("persisted: %+v", list)
	}

	for _, test := range []struct {
		method string
		body   map[string]any
		field  string
	}{
		{http.MethodPost, map[string]any{"title": "", "status": "todo"}, "title"},
		{http.MethodPost, map[string]any{"title": "Bad", "status": "unknown"}, "status"},
		{http.MethodPost, map[string]any{"title": "Bad"}, "status"},
		{http.MethodPost, map[string]any{"title": "Bad", "status": "todo", "actor_id": issue.CreatedByActorID}, "actor_id"},
		{http.MethodPost, map[string]any{"title": "Bad", "status": "todo", "assignee_actor_id": issue.CreatedByActorID}, "assignee_actor_id"},
		{http.MethodPatch, map[string]any{"status": nil}, "status"},
		{http.MethodPatch, map[string]any{"title": nil}, "title"},
		{http.MethodPatch, map[string]any{"description": "Missing revision"}, "description_revision"},
	} {
		requestPath := path
		if test.method == http.MethodPatch {
			requestPath = editPath
		}
		wantField(t, a.request(test.method, requestPath, a.agent, test.body), test.field)
	}

	a.call(http.MethodPost, "/api/projects/WP/issues", a.member, map[string]string{"title": "Another issue", "status": "todo"}, http.StatusCreated, nil)
	a.call(http.MethodPatch, "/api/issues/WP-2/subissues/"+child.ID, a.agent, map[string]string{"title": "Wrong parent"}, http.StatusNotFound, nil)
	a.call(http.MethodGet, "/api/issues/WP-99/subissues", a.member, nil, http.StatusNotFound, nil)
	a.call(http.MethodPatch, path+"/missing", a.agent, map[string]string{"status": "done"}, http.StatusNotFound, nil)
	a.call(http.MethodPatch, editPath, "", map[string]string{"status": "done"}, http.StatusUnauthorized, nil)
	for _, suffix := range []string{"comments", "reviews", "subissues"} {
		a.call(http.MethodPost, editPath+"/"+suffix, a.agent, map[string]string{"body": "No attachments"}, http.StatusNotFound, nil)
	}
	if _, err := a.store.GetIssue(context.Background(), child.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("subissue became a full issue: %v", err)
	}

	events, err := a.store.ListEvents(context.Background(), store.EventFilter{IssueID: issue.ID})
	if err != nil {
		t.Fatal(err)
	}
	var me struct {
		Actor actorView `json:"actor"`
	}
	a.call(http.MethodGet, "/api/me", a.agent, nil, http.StatusOK, &me)
	var actors []string
	for _, event := range events {
		if event.Kind == "issue.subissue_created" || event.Kind == "issue.subissue_updated" {
			actors = append(actors, event.ActorID)
		}
	}
	if len(actors) != 4 || actors[0] != me.Actor.ID || actors[1] == me.Actor.ID || actors[2] != actors[1] || actors[3] != me.Actor.ID {
		t.Fatalf("subissue actors: %v, agent %s", actors, me.Actor.ID)
	}
}
