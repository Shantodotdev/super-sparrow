package httpserver

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// upload sends raw bytes as an attachment of project WP.
func (a *apiTeam) upload(credential string, data []byte) *httptest.ResponseRecorder {
	a.t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/projects/WP/attachments", bytes.NewReader(data))
	request.Header.Set("Content-Type", "image/png")
	if strings.HasPrefix(credential, auth.TokenPrefix) {
		request.Header.Set("Authorization", "Bearer "+credential)
	} else {
		request.AddCookie(&http.Cookie{Name: "__Host-sparrow_session", Value: credential})
	}
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

func TestAttachmentsUploadAndServeImagesOnly(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	data := testPNG(t)

	response := a.upload(a.member, data)
	wantStatus(t, response, http.StatusCreated)
	var created struct {
		Attachment attachmentView `json:"attachment"`
	}
	decode(t, response, &created)
	if created.Attachment.ContentType != "image/png" || created.Attachment.URL != "/api/attachments/"+created.Attachment.ID {
		t.Fatalf("attachment %+v", created.Attachment)
	}

	served := a.request(http.MethodGet, created.Attachment.URL, a.agent, nil)
	wantStatus(t, served, http.StatusOK)
	if !bytes.Equal(served.Body.Bytes(), data) || served.Header().Get("Content-Type") != "image/png" ||
		served.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(served.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("served %v", served.Header())
	}
	wantStatus(t, a.request(http.MethodGet, created.Attachment.URL, "", nil), http.StatusUnauthorized)

	refused := a.upload(a.member, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
	wantStatus(t, refused, http.StatusBadRequest)
	if !strings.Contains(refused.Body.String(), `"field":"body"`) {
		t.Fatalf("refusal %s", refused.Body)
	}
}

func TestContextsThroughTheAPI(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	var created struct {
		Context contextView `json:"context"`
	}
	a.call(http.MethodPost, "/api/projects/WP/contexts", a.member, map[string]any{
		"title": "Checkout inspiration", "body": "Look at the stepper", "issue_id": issue.ID,
	}, http.StatusCreated, &created)
	if created.Context.Key != "WP-C1" || created.Context.IssueKey != "WP-1" || created.Context.BodyRevision != 1 {
		t.Fatalf("created %+v", created.Context)
	}

	var updated struct {
		Context contextView `json:"context"`
	}
	a.call(http.MethodPatch, "/api/contexts/wp-c1", a.agent, map[string]any{"body": "Edited", "body_revision": 1, "issue_id": nil},
		http.StatusOK, &updated)
	if updated.Context.Body != "Edited" || updated.Context.BodyRevision != 2 || updated.Context.IssueKey != "" {
		t.Fatalf("updated %+v", updated.Context)
	}
	a.call(http.MethodPatch, "/api/contexts/WP-C1", a.member, map[string]any{"body": "Stale", "body_revision": 1}, http.StatusConflict, nil)

	var listed struct {
		Contexts []contextView `json:"contexts"`
	}
	a.call(http.MethodGet, "/api/projects/WP/contexts?search=checkout", a.member, nil, http.StatusOK, &listed)
	if len(listed.Contexts) != 1 || listed.Contexts[0].Key != "WP-C1" {
		t.Fatalf("listed %+v", listed.Contexts)
	}

	a.call(http.MethodDelete, "/api/contexts/WP-C1", a.agent, nil, http.StatusForbidden, nil)
	a.call(http.MethodDelete, "/api/contexts/WP-C1", a.member, nil, http.StatusNoContent, nil)
	a.call(http.MethodGet, "/api/contexts/WP-C1", a.member, nil, http.StatusNotFound, nil)
}

func TestMCPContextsShowTheirImages(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	data := testPNG(t)
	response := a.upload(a.member, data)
	wantStatus(t, response, http.StatusCreated)
	var created struct {
		Attachment attachmentView `json:"attachment"`
	}
	decode(t, response, &created)

	saved := a.toolObject("create_context", map[string]any{
		"project_key": "WP", "title": "Broken header", "issue_key": "WP-1",
		"body": "The header overlaps.\n\n![Screenshot](" + created.Attachment.URL + ")",
	})
	if saved["key"] != "WP-C1" || saved["issue_key"] != "WP-1" || saved["created_by"].(map[string]any)["kind"] != "agent" {
		t.Fatalf("saved %+v", saved)
	}

	result := a.rpc("tools/call", map[string]any{"name": "get_context", "arguments": map[string]any{"context_key": "wp-c1"}})
	content := result["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content blocks %d", len(content))
	}
	block := content[1].(map[string]any)
	decoded, err := base64.StdEncoding.DecodeString(block["data"].(string))
	if err != nil || block["type"] != "image" || block["mimeType"] != "image/png" || !bytes.Equal(decoded, data) {
		t.Fatalf("image block %+v", block)
	}
	images := result["structuredContent"].(map[string]any)["images"].([]any)
	if len(images) != 1 || images[0].(map[string]any)["included"] != true ||
		images[0].(map[string]any)["url"] != origin+created.Attachment.URL {
		t.Fatalf("images %+v", images)
	}

	textOnly := a.rpc("tools/call", map[string]any{"name": "get_context", "arguments": map[string]any{"context_key": "WP-C1", "include_images": false}})
	if len(textOnly["content"].([]any)) != 1 {
		t.Fatal("include_images false still sent images")
	}

	listed := a.toolObject("list_contexts", map[string]any{"project_key": "WP", "issue_key": "WP-1"})
	contexts := listed["contexts"].([]any)
	if len(contexts) != 1 || contexts[0].(map[string]any)["body"] != nil {
		t.Fatalf("listed %+v", contexts)
	}

	a.mustTool("update_context", map[string]any{"context_key": "WP-C1", "issue_key": ""}, nil)
	if a.toolObject("get_context", map[string]any{"context_key": "WP-C1", "include_images": false})["issue_key"] != "" {
		t.Fatal("issue was not unlinked")
	}
}

func TestMCPIssueImagesComeFromThePlan(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	response := a.upload(a.agent, testPNG(t))
	wantStatus(t, response, http.StatusCreated)
	var created struct {
		Attachment attachmentView `json:"attachment"`
	}
	decode(t, response, &created)

	plan := a.toolObject("get_plan", map[string]any{"issue_key": "WP-1"})
	a.mustTool("update_plan", map[string]any{
		"issue_key": "WP-1", "plan_revision": plan["plan_revision"], "plan": "Bug:\n\n![shot](" + created.Attachment.URL + ")",
	}, nil)

	result := a.rpc("tools/call", map[string]any{"name": "get_issue_images", "arguments": map[string]any{"issue_key": "WP-1"}})
	content := result["content"].([]any)
	if len(content) != 2 || content[1].(map[string]any)["type"] != "image" {
		t.Fatalf("content %+v", content)
	}
}
