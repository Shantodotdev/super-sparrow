package httpserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

type beginResponse struct {
	Attachment attachmentView `json:"attachment"`
	Upload     struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
	} `json:"upload"`
}

type attachmentList struct {
	Attachments []attachmentView `json:"attachments"`
}

var (
	pngBytes = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 100)...)
	pdfBytes = []byte("%PDF-1.7\n%test")
	htmlBody = []byte("<!DOCTYPE html><html><script>alert(1)</script></html>")
)

func attachmentsPath(issueKey string) string { return "/api/issues/" + issueKey + "/attachments" }

// begin starts an attachment and checks the answer.
func (a *apiTeam) begin(issueKey, credential, filename, contentType string, size int) beginResponse {
	a.t.Helper()

	var begun beginResponse
	a.call(http.MethodPost, attachmentsPath(issueKey), credential, map[string]any{
		"filename": filename, "content_type": contentType, "byte_size": size,
	}, http.StatusCreated, &begun)

	return begun
}

// attach runs the whole flow: begin, the browser's PUT to the bucket, and complete.
func (a *apiTeam) attach(issueKey, credential, filename, contentType string, data []byte) attachmentView {
	a.t.Helper()

	begun := a.begin(issueKey, credential, filename, contentType, len(data))
	a.objects.Put(begun.Attachment.ID, data)

	var completed struct {
		Attachment attachmentView `json:"attachment"`
	}
	a.call(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", credential, nil, http.StatusOK, &completed)

	return completed.Attachment
}

// redirect returns the signed link a download answers with.
func (a *apiTeam) redirect(path, credential string) (*httptest.ResponseRecorder, *url.URL) {
	a.t.Helper()

	response := a.request(http.MethodGet, path, credential, nil)
	wantStatus(a.t, response, http.StatusFound)
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		a.t.Fatalf("parse Location %q: %v", response.Header().Get("Location"), err)
	}

	return response, location
}

func TestAttachmentBeginPutCompleteListDownloadAndDelete(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	begun := a.begin(issue.Key, a.agent, "shot.png", "image/png", len(pngBytes))
	view := begun.Attachment
	if view.Filename != "shot.png" || view.ContentType != "image/png" || view.ByteSize != int64(len(pngBytes)) ||
		view.URL != "/api/attachments/"+view.ID || view.IssueKey != issue.Key || view.ProjectKey != "WP" {
		t.Fatalf("begun view %+v", view)
	}
	if begun.Upload.Method != http.MethodPut || begun.Upload.Headers["Content-Type"] != "image/png" ||
		begun.Upload.URL == "" {
		t.Fatalf("upload %+v", begun.Upload)
	}

	// Pending: hidden from the list and the download, and not completable before the PUT.
	var listed attachmentList
	a.call(http.MethodGet, attachmentsPath(issue.Key), a.owner, nil, http.StatusOK, &listed)
	if len(listed.Attachments) != 0 {
		t.Fatalf("pending attachment listed: %+v", listed)
	}
	wantStatus(t, a.request(http.MethodGet, view.URL, a.owner, nil), http.StatusNotFound)
	wantStatus(t, a.request(http.MethodPost, "/api/attachments/"+view.ID+"/complete", a.agent, nil), http.StatusBadRequest)

	// The browser's PUT goes straight to the bucket; then the agent confirms it.
	a.objects.Put(view.ID, pngBytes)
	var completed struct {
		Attachment attachmentView `json:"attachment"`
	}
	a.call(http.MethodPost, "/api/attachments/"+view.ID+"/complete", a.agent, nil, http.StatusOK, &completed)
	if completed.Attachment.ID != view.ID || completed.Attachment.ContentType != "image/png" || completed.Attachment.ByteSize != int64(len(pngBytes)) {
		t.Fatalf("completed %+v", completed.Attachment)
	}

	a.call(http.MethodGet, attachmentsPath(issue.Key), a.owner, nil, http.StatusOK, &listed)
	if len(listed.Attachments) != 1 || listed.Attachments[0].ID != view.ID {
		t.Fatalf("listed %+v", listed)
	}

	// Images are shown inline, through a private redirect to the signed link.
	response, location := a.redirect(view.URL, a.owner)
	query := location.Query()
	if location.Host != "objects.test" || query.Get("type") != "image/png" || query.Get("disposition") != "inline" ||
		response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("redirect %v, headers %v", location, response.Header())
	}

	// An agent can read but not delete.
	a.redirect(view.URL, a.agent)
	a.call(http.MethodDelete, view.URL, a.agent, nil, http.StatusForbidden, nil)
	a.call(http.MethodDelete, view.URL, a.member, nil, http.StatusNoContent, nil)
	wantStatus(t, a.request(http.MethodGet, view.URL, a.owner, nil), http.StatusNotFound)
	if a.objects.Has(view.ID) {
		t.Fatal("deleted attachment's object is still in the bucket")
	}
}

func TestDownloadsOfRiskyTypesAreForcedToDownload(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	for name, test := range map[string]struct {
		data        []byte
		declared    string
		wantType    string
		wantInline  bool
		wantFilePat string
	}{
		"page.html":        {htmlBody, "text/html", "text/html", false, "page.html"},
		"logo.svg":         {[]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), "image/svg+xml", "", false, "logo.svg"},
		"run.js":           {[]byte("alert(1)"), "text/javascript", "", false, "run.js"},
		"résumé \"x\".pdf": {pdfBytes, "application/pdf", "application/pdf", true, ""},
	} {
		t.Run(name, func(t *testing.T) {
			view := a.attach(issue.Key, a.member, name, test.declared, test.data)
			_, location := a.redirect(view.URL, a.member)
			query := location.Query()

			if test.wantInline {
				if query.Get("type") != test.wantType || query.Get("disposition") != "inline" {
					t.Fatalf("inline redirect %v", location)
				}
				return
			}
			// The in-memory bucket puts the disposition in the query unescaped, so compare it as written.
			want := "type=application/octet-stream&disposition=attachment; filename*=UTF-8''" + test.wantFilePat
			if location.RawQuery != want {
				t.Fatalf("forced download query %q, want %q", location.RawQuery, want)
			}
		})
	}
}

func TestDownloadEncodesTheFilename(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	view := a.attach(issue.Key, a.member, "résumé \"x\".txt", "text/plain", []byte("hello"))
	_, location := a.redirect(view.URL, a.member)

	want := "type=application/octet-stream&disposition=attachment; filename*=UTF-8''r%C3%A9sum%C3%A9%20%22x%22.txt"
	if location.RawQuery != want {
		t.Fatalf("query %q, want %q", location.RawQuery, want)
	}
}

func TestVideoIsShownInline(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()
	video := append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), []byte("0123456789")...)

	view := a.attach(issue.Key, a.member, "clip.mp4", "video/mp4", video)
	_, location := a.redirect(view.URL, a.member)

	if location.Query().Get("type") != "video/mp4" || location.Query().Get("disposition") != "inline" {
		t.Fatalf("redirect %v", location)
	}
}

func TestAttachmentRequestsAreChecked(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()
	path := attachmentsPath(issue.Key)
	const missing = "/api/attachments/01900000-0000-7000-8000-000000000000"

	valid := map[string]any{"filename": "a.txt", "content_type": "text/plain", "byte_size": 5}
	with := func(field string, value any) map[string]any {
		body := map[string]any{}
		for k, v := range valid {
			body[k] = v
		}
		body[field] = value
		return body
	}

	// Anonymous callers.
	wantStatus(t, a.request(http.MethodPost, path, "", valid), http.StatusUnauthorized)
	wantStatus(t, a.request(http.MethodGet, path, "", nil), http.StatusUnauthorized)
	wantStatus(t, a.request(http.MethodGet, missing, "", nil), http.StatusUnauthorized)
	wantStatus(t, a.request(http.MethodPost, missing+"/complete", "", nil), http.StatusUnauthorized)
	wantStatus(t, a.request(http.MethodDelete, missing, "", nil), http.StatusUnauthorized)

	// Bad bodies.
	wantStatus(t, a.request(http.MethodPost, path, a.member, "not an object"), http.StatusBadRequest)
	wantStatus(t, a.request(http.MethodPost, path, a.member, with("filename", "")), http.StatusBadRequest)
	wantStatus(t, a.request(http.MethodPost, path, a.member, with("filename", "..")), http.StatusBadRequest)
	wantStatus(t, a.request(http.MethodPost, path, a.member, with("byte_size", 0)), http.StatusBadRequest)
	wantStatus(t, a.request(http.MethodPost, path, a.member, with("byte_size", -3)), http.StatusBadRequest)
	wantStatus(t, a.request(http.MethodPost, path, a.member, with("byte_size", 100_000_001)), http.StatusRequestEntityTooLarge)
	wantStatus(t, a.request(http.MethodPost, path, a.member, map[string]any{"filename": "v.mp4", "content_type": "video/mp4", "byte_size": 5_000_000_001}), http.StatusRequestEntityTooLarge)
	wantField(t, a.request(http.MethodPost, path, a.member, with("filename", "")), "filename")

	// Unknown things.
	wantStatus(t, a.request(http.MethodPost, attachmentsPath("WP-99"), a.member, valid), http.StatusNotFound)
	wantStatus(t, a.request(http.MethodGet, "/api/attachments/not-an-id", a.member, nil), http.StatusNotFound)
	wantStatus(t, a.request(http.MethodGet, missing, a.member, nil), http.StatusNotFound)
	wantStatus(t, a.request(http.MethodPost, missing+"/complete", a.member, nil), http.StatusNotFound)

	// Nothing was left behind by the refused requests.
	var listed attachmentList
	a.call(http.MethodGet, path, a.owner, nil, http.StatusOK, &listed)
	if len(listed.Attachments) != 0 {
		t.Fatalf("attachments %+v", listed.Attachments)
	}
}

func TestOnlyTheBeginnerCanComplete(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	begun := a.begin(issue.Key, a.agent, "a.txt", "text/plain", 5)
	a.objects.Put(begun.Attachment.ID, []byte("hello"))

	wantStatus(t, a.request(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.owner, nil), http.StatusNotFound)
	a.call(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.agent, nil, http.StatusOK, nil)
	// Completing again returns the same attachment.
	a.call(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.agent, nil, http.StatusOK, nil)
}

func TestCompletingAnEmptyUploadDiscardsIt(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	begun := a.begin(issue.Key, a.member, "a.txt", "text/plain", 5)
	a.objects.Put(begun.Attachment.ID, nil)

	wantStatus(t, a.request(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.member, nil), http.StatusBadRequest)
	wantStatus(t, a.request(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.member, nil), http.StatusNotFound)
	if a.objects.Has(begun.Attachment.ID) {
		t.Fatal("the empty object was kept")
	}
}

func TestBeginnerCanAbandonAPendingAttachment(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	begun := a.begin(issue.Key, a.member, "a.txt", "text/plain", 5)
	a.call(http.MethodDelete, begun.Attachment.URL, a.member, nil, http.StatusNoContent, nil)
	wantStatus(t, a.request(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.member, nil), http.StatusNotFound)
}

func TestAttachmentEventsReachTheStream(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	var before struct {
		NextAfter int64 `json:"next_after"`
	}
	a.call(http.MethodGet, "/api/events?limit=200", a.owner, nil, http.StatusOK, &before)

	// Beginning is silent; the event comes with completion.
	begun := a.begin(issue.Key, a.member, "a.txt", "text/plain", 5)
	a.objects.Put(begun.Attachment.ID, []byte("hello"))
	a.call(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.member, nil, http.StatusOK, nil)

	var after struct {
		Events []struct {
			Kind    string         `json:"kind"`
			Payload map[string]any `json:"payload"`
		} `json:"events"`
	}
	a.call(http.MethodGet, "/api/events?after="+strconv.FormatInt(before.NextAfter, 10), a.owner, nil, http.StatusOK, &after)
	if len(after.Events) != 1 || after.Events[0].Kind != "attachment.created" || after.Events[0].Payload["issue_key"] != issue.Key {
		t.Fatalf("events %+v", after.Events)
	}
}

func TestSubissueAndFindingAttachmentsAreListedSeparately(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	var created struct {
		Subissue subissueView `json:"subissue"`
	}
	a.call(http.MethodPost, "/api/issues/"+issue.Key+"/subissues", a.member, map[string]any{"title": "Child", "status": "todo"}, http.StatusCreated, &created)
	subissuePath := "/api/issues/" + issue.Key + "/subissues/" + created.Subissue.ID + "/attachments"

	var begun beginResponse
	a.call(http.MethodPost, subissuePath, a.member, map[string]any{"filename": "child.png", "content_type": "image/png", "byte_size": len(pngBytes)}, http.StatusCreated, &begun)
	if begun.Attachment.SubissueID != created.Subissue.ID || begun.Attachment.FindingID != "" {
		t.Fatalf("begun %+v", begun.Attachment)
	}
	a.objects.Put(begun.Attachment.ID, pngBytes)
	a.call(http.MethodPost, "/api/attachments/"+begun.Attachment.ID+"/complete", a.member, nil, http.StatusOK, nil)

	var onSubissue, onIssue attachmentList
	a.call(http.MethodGet, subissuePath, a.owner, nil, http.StatusOK, &onSubissue)
	a.call(http.MethodGet, attachmentsPath(issue.Key), a.owner, nil, http.StatusOK, &onIssue)
	if len(onSubissue.Attachments) != 1 || onSubissue.Attachments[0].ID != begun.Attachment.ID || len(onIssue.Attachments) != 0 {
		t.Fatalf("subissue list %+v, issue list %+v", onSubissue, onIssue)
	}

	a.call(http.MethodPost, "/api/issues/"+issue.Key+"/subissues/missing/attachments", a.member,
		map[string]any{"filename": "x.png", "content_type": "image/png", "byte_size": 5}, http.StatusNotFound, nil)
}
