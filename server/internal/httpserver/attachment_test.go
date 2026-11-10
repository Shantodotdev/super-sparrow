package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
)

// upload sends a raw file body to the issue's attachments, as the credential's owner.
func (a *apiTeam) upload(path, credential string, body io.Reader) *httptest.ResponseRecorder {
	a.t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0")
	if strings.HasPrefix(credential, auth.TokenPrefix) {
		request.Header.Set("Authorization", "Bearer "+credential)
	} else if credential != "" {
		request.AddCookie(&http.Cookie{Name: "__Host-sparrow_session", Value: credential})
	}

	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

func (a *apiTeam) get(path, credential string, header map[string]string) *httptest.ResponseRecorder {
	a.t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0")
	for name, value := range header {
		request.Header.Set(name, value)
	}
	if strings.HasPrefix(credential, auth.TokenPrefix) {
		request.Header.Set("Authorization", "Bearer "+credential)
	} else if credential != "" {
		request.AddCookie(&http.Cookie{Name: "__Host-sparrow_session", Value: credential})
	}

	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

type attachmentBody struct {
	Attachment attachmentView `json:"attachment"`
}

func TestAttachmentsUploadListDownloadAndDelete(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()
	png := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 100)...)

	response := a.upload("/api/issues/"+issue.Key+"/attachments?filename=shot.png", a.agent, bytes.NewReader(png))
	wantStatus(t, response, http.StatusCreated)
	var created attachmentBody
	decode(t, response, &created)
	sum := sha256.Sum256(png)
	view := created.Attachment
	if view.Filename != "shot.png" || view.ContentType != "image/png" || view.ByteSize != int64(len(png)) ||
		view.SHA256 != hex.EncodeToString(sum[:]) || view.URL != "/api/attachments/"+view.ID || view.IssueKey != issue.Key || view.ProjectKey != "WP" {
		t.Fatalf("view %+v", view)
	}

	var listed struct {
		Attachments []attachmentView `json:"attachments"`
	}
	a.call(http.MethodGet, "/api/issues/"+issue.Key+"/attachments", a.owner, nil, http.StatusOK, &listed)
	if len(listed.Attachments) != 1 || listed.Attachments[0].ID != view.ID {
		t.Fatalf("listed %+v", listed)
	}

	// Images are shown inline, with the headers that keep a browser from guessing.
	file := a.get(view.URL, a.owner, nil)
	wantStatus(t, file, http.StatusOK)
	if !bytes.Equal(file.Body.Bytes(), png) || file.Header().Get("Content-Type") != "image/png" ||
		file.Header().Get("Content-Disposition") != "inline" || file.Header().Get("X-Content-Type-Options") != "nosniff" ||
		file.Header().Get("Cache-Control") != "private, no-cache" || file.Header().Get("ETag") != `"`+view.SHA256+`"` ||
		!strings.Contains(file.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("headers %v", file.Header())
	}

	// An agent can read but not delete.
	wantStatus(t, a.get(view.URL, a.agent, nil), http.StatusOK)
	a.call(http.MethodDelete, view.URL, a.agent, nil, http.StatusForbidden, nil)
	a.call(http.MethodDelete, view.URL, a.member, nil, http.StatusNoContent, nil)
	wantStatus(t, a.get(view.URL, a.owner, nil), http.StatusNotFound)
}

func TestAttachmentRangeRequests(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()
	video := append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), []byte("0123456789")...)

	var created attachmentBody
	response := a.upload("/api/issues/"+issue.Key+"/attachments?filename=clip.mp4", a.member, bytes.NewReader(video))
	wantStatus(t, response, http.StatusCreated)
	decode(t, response, &created)

	partial := a.get(created.Attachment.URL, a.member, map[string]string{"Range": "bytes=-10"})
	wantStatus(t, partial, http.StatusPartialContent)
	if partial.Body.String() != "0123456789" || partial.Header().Get("Content-Type") != "video/mp4" ||
		partial.Header().Get("Accept-Ranges") != "bytes" || partial.Header().Get("Content-Disposition") != "inline" {
		t.Fatalf("range response %q %v", partial.Body, partial.Header())
	}

	revalidated := a.get(created.Attachment.URL, a.member, map[string]string{"If-None-Match": `"` + created.Attachment.SHA256 + `"`})
	wantStatus(t, revalidated, http.StatusNotModified)
}

func TestDangerousTypesAreDownloadsOnly(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	for name, content := range map[string]string{
		"page.html": "<!DOCTYPE html><html><script>alert(1)</script></html>",
		"logo.svg":  `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"run.js":    "alert(1)",
	} {
		t.Run(name, func(t *testing.T) {
			var created attachmentBody
			response := a.upload("/api/issues/"+issue.Key+"/attachments?filename="+name, a.member, strings.NewReader(content))
			wantStatus(t, response, http.StatusCreated)
			decode(t, response, &created)

			file := a.get(created.Attachment.URL, a.member, nil)
			wantStatus(t, file, http.StatusOK)
			if file.Header().Get("Content-Type") != "application/octet-stream" ||
				file.Header().Get("Content-Disposition") != "attachment; filename*=UTF-8''"+name {
				t.Fatalf("headers %v", file.Header())
			}
		})
	}

	var created attachmentBody
	response := a.upload("/api/issues/"+issue.Key+"/attachments?filename="+"r%C3%A9sum%C3%A9%20%22x%22.pdf", a.member, strings.NewReader("%PDF-1.7"))
	wantStatus(t, response, http.StatusCreated)
	decode(t, response, &created)
	file := a.get(created.Attachment.URL, a.member, nil)
	if file.Header().Get("Content-Type") != "application/pdf" || file.Header().Get("Content-Disposition") != "inline" {
		t.Fatalf("pdf headers %v", file.Header())
	}
}

func TestAttachmentRequestsAreChecked(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()
	path := "/api/issues/" + issue.Key + "/attachments"

	wantStatus(t, a.upload(path+"?filename=a.txt", "", strings.NewReader("x")), http.StatusUnauthorized)
	wantStatus(t, a.get(path, "", nil), http.StatusUnauthorized)
	wantStatus(t, a.get("/api/attachments/01900000-0000-7000-8000-000000000000", "", nil), http.StatusUnauthorized)
	wantStatus(t, a.upload(path, a.member, strings.NewReader("x")), http.StatusBadRequest)
	wantStatus(t, a.upload(path+"?filename=..", a.member, strings.NewReader("x")), http.StatusBadRequest)
	wantStatus(t, a.upload(path+"?filename=a.txt", a.member, strings.NewReader("")), http.StatusBadRequest)
	wantStatus(t, a.upload("/api/issues/WP-99/attachments?filename=a.txt", a.member, strings.NewReader("x")), http.StatusNotFound)
	wantStatus(t, a.get("/api/attachments/not-an-id", a.member, nil), http.StatusNotFound)
	wantStatus(t, a.get("/api/attachments/01900000-0000-7000-8000-000000000000", a.member, nil), http.StatusNotFound)
}

func TestAttachmentOverTheLimitIsRefusedBeforeReading(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	request := httptest.NewRequest(http.MethodPost, "/api/issues/"+issue.Key+"/attachments?filename=huge.bin", &neverRead{t: t})
	request.ContentLength = 5_000_000_001
	request.Header.Set("Authorization", "Bearer "+a.agent)
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)

	wantStatus(t, response, http.StatusRequestEntityTooLarge)
}

type neverRead struct{ t *testing.T }

func (r *neverRead) Read([]byte) (int, error) {
	r.t.Error("the body was read although Content-Length was over the limit")
	return 0, io.EOF
}

func TestOversizedStreamedUploadIsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("streams over 100 MB")
	}
	a := newAPITeam(t)
	issue := a.project()

	// No Content-Length is known, so the store enforces the limit while streaming.
	body := io.MultiReader(strings.NewReader("plain text"), io.LimitReader(zeroReader{}, 100_000_000))
	request := httptest.NewRequest(http.MethodPost, "/api/issues/"+issue.Key+"/attachments?filename=big.txt", body)
	request.ContentLength = -1
	request.Header.Set("Authorization", "Bearer "+a.agent)
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)

	wantStatus(t, response, http.StatusRequestEntityTooLarge)
	var listed struct {
		Attachments []attachmentView `json:"attachments"`
	}
	a.call(http.MethodGet, "/api/issues/"+issue.Key+"/attachments", a.owner, nil, http.StatusOK, &listed)
	if len(listed.Attachments) != 0 {
		t.Fatalf("attachments %+v", listed.Attachments)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestAttachmentEventsReachTheStream(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()

	var before struct {
		NextAfter int64 `json:"next_after"`
	}
	a.call(http.MethodGet, "/api/events?limit=200", a.owner, nil, http.StatusOK, &before)

	wantStatus(t, a.upload("/api/issues/"+issue.Key+"/attachments?filename=a.txt", a.member, strings.NewReader("hello")), http.StatusCreated)

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
