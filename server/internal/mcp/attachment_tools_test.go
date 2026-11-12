package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/internal/store/storetest"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

type attachmentFixture struct {
	t       *testing.T
	ctx     context.Context
	store   *store.Store
	objects *storetest.Objects
	server  *Server
	agent   store.Caller
	first   store.Issue
	second  store.Issue
}

func newAttachmentFixture(t *testing.T) *attachmentFixture {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}

	objects := storetest.NewObjects()
	st := store.New(db, time.Now, objects)
	system := store.Caller{ActorID: store.SystemActorID}
	owner, err := st.CreateHuman(ctx, system, store.NewHuman{DisplayName: "Olive Owner", Email: "olive@example.com", Role: store.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := st.CreateAgent(ctx, store.Caller{ActorID: owner.ID}, store.NewAgent{DisplayName: "Claude Code", AgentName: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}

	f := &attachmentFixture{t: t, ctx: ctx, store: st, objects: objects, agent: store.Caller{ActorID: agent.ID}}
	human := store.Caller{ActorID: owner.ID}
	for i, key := range []string{"WP", "OT"} {
		project, err := st.CreateProject(ctx, human, store.NewProject{Key: key, Name: "Project " + key})
		if err != nil {
			t.Fatal(err)
		}
		issue, err := st.CreateIssue(ctx, human, store.NewIssue{ProjectID: project.ID, Title: "Issue", Status: store.IssueTodo})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			f.first = issue
		} else {
			f.second = issue
		}
	}

	publicURL, _ := url.Parse("https://sparrow.example.com")
	f.server = New(st, publicURL, slog.New(slog.NewTextHandler(io.Discard, nil)), "test")
	return f
}

func (f *attachmentFixture) attach(issue store.Issue, name string, data []byte) store.Attachment {
	f.t.Helper()
	attachment, _, err := f.store.BeginAttachment(f.ctx, f.agent, issue.Key(), name, "application/octet-stream", int64(len(data)))
	if err != nil {
		f.t.Fatal(err)
	}
	f.objects.Put(attachment.ID, data)

	attachment, err = f.store.CompleteAttachment(f.ctx, f.agent, attachment.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return attachment
}

// call runs a tool through the JSON-RPC handler and returns the result object.
func (f *attachmentFixture) call(name string, args map[string]any) map[string]any {
	f.t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}})
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	f.server.ServeHTTP(recorder, request, f.agent)

	var answer struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.Result == nil {
		f.t.Fatalf("%s: %s", name, recorder.Body.String())
	}
	return answer.Result
}

// content returns the result's content blocks.
func content(result map[string]any) []map[string]any {
	var blocks []map[string]any
	for _, block := range result["content"].([]any) {
		blocks = append(blocks, block.(map[string]any))
	}
	return blocks
}

// metadata decodes the JSON in the first content block.
func metadata(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(content(result)[0]["text"].(string)), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func pngData() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)
}

func TestListAttachmentsOldestFirstForOneIssue(t *testing.T) {
	f := newAttachmentFixture(t)
	older := f.attach(f.first, "shot.png", pngData())
	newer := f.attach(f.first, "notes.txt", []byte("hello"))
	f.attach(f.second, "other.txt", []byte("elsewhere"))

	result := f.call("list_attachments", map[string]any{"issue_key": f.first.Key()})
	if result["isError"] != false {
		t.Fatalf("result %v", result)
	}
	listed := metadata(t, result)["attachments"].([]any)
	if len(listed) != 2 {
		t.Fatalf("listed %d attachments", len(listed))
	}

	first, second := listed[0].(map[string]any), listed[1].(map[string]any)
	if first["id"] != older.ID || second["id"] != newer.ID {
		t.Fatalf("order %v", listed)
	}
	creator := first["created_by"].(map[string]any)
	if first["filename"] != "shot.png" || first["content_type"] != "image/png" ||
		first["byte_size"].(float64) != float64(older.ByteSize) || first["url"] != "https://sparrow.example.com/api/attachments/"+older.ID ||
		first["created_at"] == "" || creator["name"] != "Claude Code" || creator["kind"] != "agent" {
		t.Fatalf("attachment %v", first)
	}
}

func TestListAttachmentsHidesPendingUploads(t *testing.T) {
	f := newAttachmentFixture(t)
	done := f.attach(f.first, "done.txt", []byte("hello"))
	pending, _, err := f.store.BeginAttachment(f.ctx, f.agent, f.first.Key(), "pending.txt", "text/plain", 5)
	if err != nil {
		t.Fatal(err)
	}

	result := f.call("list_attachments", map[string]any{"issue_key": f.first.Key()})
	listed := metadata(t, result)["attachments"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["id"] != done.ID {
		t.Fatalf("listed %v", listed)
	}

	result = f.call("get_attachment", map[string]any{"attachment_id": pending.ID})
	if result["isError"] != true {
		t.Fatalf("pending attachment was readable: %v", result)
	}
}

func TestListAttachmentsRefusesUnknownIssue(t *testing.T) {
	f := newAttachmentFixture(t)

	result := f.call("list_attachments", map[string]any{"issue_key": "NOPE-9"})
	if result["isError"] != true {
		t.Fatalf("result %v", result)
	}
}

func TestGetAttachmentShowsImage(t *testing.T) {
	f := newAttachmentFixture(t)
	data := pngData()
	attachment := f.attach(f.first, "shot.png", data)

	result := f.call("get_attachment", map[string]any{"attachment_id": attachment.ID})
	blocks := content(result)
	meta := metadata(t, result)
	if len(blocks) != 2 || meta["content_included"] != true || meta["id"] != attachment.ID || meta["issue_key"] != f.first.Key() {
		t.Fatalf("result %v", result)
	}
	if blocks[1]["type"] != "image" || blocks[1]["mimeType"] != "image/png" || blocks[1]["data"] != base64.StdEncoding.EncodeToString(data) {
		t.Fatalf("image block %v", blocks[1])
	}
}

func TestGetAttachmentWithoutContentReturnsMetadataOnly(t *testing.T) {
	f := newAttachmentFixture(t)
	attachment := f.attach(f.first, "shot.png", pngData())

	result := f.call("get_attachment", map[string]any{"attachment_id": attachment.ID, "include_content": false})
	meta := metadata(t, result)
	if len(content(result)) != 1 || meta["content_included"] != false || meta["content_reason"] == "" {
		t.Fatalf("result %v", result)
	}
}

func TestGetAttachmentShowsTextAndMarksTruncation(t *testing.T) {
	f := newAttachmentFixture(t)
	short := f.attach(f.first, "notes.txt", []byte("héllo\nworld"))
	long := f.attach(f.first, "long.txt", []byte(strings.Repeat("é", maxInlineTextBytes)))

	result := f.call("get_attachment", map[string]any{"attachment_id": short.ID})
	blocks := content(result)
	if len(blocks) != 2 || blocks[1]["type"] != "text" || blocks[1]["text"] != "héllo\nworld" || metadata(t, result)["content_included"] != true {
		t.Fatalf("short text %v", result)
	}

	// The cut lands inside a two-byte character, which is dropped rather than shown broken.
	result = f.call("get_attachment", map[string]any{"attachment_id": long.ID})
	blocks = content(result)
	shown := blocks[1]["text"].(string)
	meta := metadata(t, result)
	if meta["content_included"] != true || meta["content_truncated"] != true || !strings.Contains(shown, "[truncated") {
		t.Fatalf("long text %v", meta)
	}
	if body, _, _ := strings.Cut(shown, "\n[truncated"); len(body) != maxInlineTextBytes || strings.Contains(body, "�") {
		t.Fatalf("shown %d bytes", len(body))
	}
}

func TestGetAttachmentReturnsMetadataOnlyForOtherFiles(t *testing.T) {
	f := newAttachmentFixture(t)
	video := f.attach(f.first, "clip.mp4", append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, 100)...))
	binary := f.attach(f.first, "notes.txt", []byte("text\x00with a nul"))
	invalid := f.attach(f.first, "latin.txt", []byte("caf\xe9 au lait"))

	for _, attachment := range []store.Attachment{video, binary, invalid} {
		result := f.call("get_attachment", map[string]any{"attachment_id": attachment.ID})
		meta := metadata(t, result)
		if len(content(result)) != 1 || meta["content_included"] != false || meta["content_reason"] == "" {
			t.Fatalf("%s: %v", attachment.Filename, result)
		}
	}
}

func TestGetAttachmentRefusesUnknownAndInvalidIDs(t *testing.T) {
	f := newAttachmentFixture(t)

	for _, id := range []string{"missing", " ", "../../etc/passwd"} {
		result := f.call("get_attachment", map[string]any{"attachment_id": id})
		if result["isError"] != true {
			t.Fatalf("%q: %v", id, result)
		}
	}
}

func TestAttachmentToolsAreReadOnly(t *testing.T) {
	f := newAttachmentFixture(t)

	for _, name := range []string{"list_attachments", "get_attachment"} {
		found := false
		for _, tool := range f.server.tools {
			if tool.name == name {
				found = tool.readOnly
			}
		}
		if !found {
			t.Fatalf("%s is not a read-only tool", name)
		}
	}
	for _, tool := range f.server.tools {
		if strings.Contains(tool.name, "attachment") && !tool.readOnly {
			t.Fatalf("%s can write", tool.name)
		}
	}
}

func TestListAttachmentsOfASubissue(t *testing.T) {
	f := newAttachmentFixture(t)
	issue := f.first
	sub, err := f.store.CreateSubissue(f.ctx, f.agent, issue.ID, store.NewSubissue{Title: "Child", Status: store.IssueTodo})
	if err != nil {
		t.Fatal(err)
	}
	pending, _, err := f.store.BeginTargetAttachment(f.ctx, f.agent, issue.Key(), store.AttachmentTarget{SubissueID: sub.ID}, "child.txt", "text/plain", 5)
	if err != nil {
		t.Fatal(err)
	}
	f.objects.Put(pending.ID, []byte("hello"))
	if _, err := f.store.CompleteAttachment(f.ctx, f.agent, pending.ID); err != nil {
		t.Fatal(err)
	}

	own := metadata(t, f.call("list_attachments", map[string]any{"issue_key": issue.Key(), "subissue_key": issue.Key() + ".1"}))
	if list, _ := own["attachments"].([]any); len(list) != 1 {
		t.Fatalf("subissue list %v, want child.txt", own)
	}
	onIssue := metadata(t, f.call("list_attachments", map[string]any{"issue_key": issue.Key()}))
	if list, _ := onIssue["attachments"].([]any); len(list) != 0 {
		t.Fatalf("issue list %v, want none", onIssue)
	}
}
