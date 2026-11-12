package store

import (
	"strings"
	"testing"
)

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)
}

func mp4Bytes(size int) []byte {
	return append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, size)...)
}

// beginAndUpload begins an attachment and simulates the finished upload of data.
func (tm *team) beginAndUpload(t *testing.T, by Caller, issueKey, filename string, data []byte) Attachment {
	t.Helper()
	attachment, _, err := tm.store.BeginAttachment(tm.ctx, by, issueKey, filename, "application/octet-stream", int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	tm.objects.Put(attachment.ID, data)
	return attachment
}

func TestBeginAttachmentReturnsPendingAttachmentAndUploadLink(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	before := tm.eventCount(t)

	attachment, upload, err := tm.store.BeginAttachment(tm.ctx, tm.agent, issue.Key(), "../../etc/shot\x00.png", "image/png; charset=binary", 1234)
	if err != nil {
		t.Fatal(err)
	}

	if !attachment.Pending || attachment.Filename != "shot.png" || attachment.ByteSize != 1234 || attachment.CreatedByID != tm.agent.ActorID ||
		attachment.IssueKey != issue.Key() || attachment.ProjectKey != "WP" || attachment.URLPath() != "/api/attachments/"+attachment.ID {
		t.Fatalf("attachment %+v", attachment)
	}
	if upload.URL != "https://objects.test/put/"+attachment.ID+"?type=image/png" || len(upload.Headers) != 1 || upload.Headers["Content-Type"] != "image/png" {
		t.Fatalf("upload %+v", upload)
	}
	if tm.eventCount(t) != before {
		t.Fatal("beginning an upload wrote an event")
	}
}

func TestBeginAttachmentUsesGenericTypeWhenDeclaredTypeIsMalformed(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")

	_, upload, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "a.bin", "not a type", 10)
	if err != nil || upload.Headers["Content-Type"] != "application/octet-stream" {
		t.Fatalf("upload %+v (%v)", upload, err)
	}
}

func TestBeginAttachmentRefusesBadInput(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	before := tm.eventCount(t)

	for _, name := range []string{"", "   ", "..", "dir/", "\x00\x01", "bad\xff.txt"} {
		_, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), name, "text/plain", 1)
		wantInvalid(t, err, "filename")
	}
	for _, size := range []int64{0, -5} {
		_, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "a.txt", "text/plain", size)
		wantInvalid(t, err, "byte_size")
	}
	_, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, "WP-99", "a.txt", "text/plain", 1)
	wantErr(t, err, ErrNotFound)
	_, _, err = tm.store.BeginAttachment(tm.ctx, tm.member, "nonsense", "a.txt", "text/plain", 1)
	wantInvalid(t, err, "key")

	var rows int
	if err := tm.store.db.QueryRow(`SELECT count(*) FROM attachments`).Scan(&rows); err != nil || rows != 0 || tm.eventCount(t) != before {
		t.Fatalf("refused begins left %d rows (%v) or events", rows, err)
	}
}

func TestBeginAttachmentSizeLimits(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")

	if _, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "big.bin", "application/octet-stream", MaxAttachmentBytes); err != nil {
		t.Fatalf("file of exactly the limit: %v", err)
	}
	_, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "big.bin", "application/octet-stream", MaxAttachmentBytes+1)
	wantErr(t, err, ErrTooLarge)
	_, _, err = tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "huge.png", "image/png", MaxVideoBytes)
	wantErr(t, err, ErrTooLarge)

	if _, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "clip.mp4", "video/mp4", MaxVideoBytes); err != nil {
		t.Fatalf("video of exactly the video limit: %v", err)
	}
	_, _, err = tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "clip.mp4", "video/mp4", MaxVideoBytes+1)
	wantErr(t, err, ErrTooLarge)
}

func TestCompleteAttachmentSniffsTypeAndRecordsActorAndEvent(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	data := pngBytes()

	// The declared type and size are wrong; the object decides both.
	pending, _, err := tm.store.BeginAttachment(tm.ctx, tm.agent, issue.Key(), "shot.png", "text/plain", 5)
	if err != nil {
		t.Fatal(err)
	}
	tm.objects.Put(pending.ID, data)

	attachment, err := tm.store.CompleteAttachment(tm.ctx, tm.agent, pending.ID)
	if err != nil {
		t.Fatal(err)
	}

	if attachment.Pending || attachment.ContentType != "image/png" || attachment.ByteSize != int64(len(data)) || attachment.CreatedByID != tm.agent.ActorID {
		t.Fatalf("attachment %+v", attachment)
	}
	stored := got(tm.store.GetAttachment(tm.ctx, pending.ID)).must(t)
	if stored.ContentType != "image/png" || stored.ByteSize != int64(len(data)) {
		t.Fatalf("stored %+v", stored)
	}

	event := tm.lastEvent(t)
	if event.Kind != "attachment.created" || event.ActorID != tm.agent.ActorID ||
		!strings.Contains(string(event.Payload), `"issue_key":"WP-1"`) || !strings.Contains(string(event.Payload), `"filename":"shot.png"`) {
		t.Fatalf("event %+v", event)
	}

	list := got(tm.store.ListIssueAttachments(tm.ctx, issue.Key())).must(t)
	if len(list) != 1 || list[0].ID != attachment.ID {
		t.Fatalf("list %v", list)
	}
}

func TestCompleteAttachmentSniffsTheContentType(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")

	cases := map[string]struct {
		data []byte
		want string
	}{
		"html":      {[]byte("<!DOCTYPE html><html><script>alert(1)</script></html>"), "text/html"},
		"text":      {[]byte("hello"), "text/plain"},
		"pdf":       {[]byte("%PDF-1.7 rest"), "application/pdf"},
		"mp4":       {mp4Bytes(10), "video/mp4"},
		"quicktime": {append([]byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00qt  "), 0), "video/quicktime"},
		"webm":      {append([]byte{0x1A, 0x45, 0xDF, 0xA3}, make([]byte, 20)...), "video/webm"},
		"binary":    {[]byte{0, 1, 2, 3, 0xff, 0xfe}, "application/octet-stream"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// The filename claims an image; the bytes decide.
			pending := tm.beginAndUpload(t, tm.member, issue.Key(), "picture.png", c.data)
			attachment, err := tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID)
			if err != nil || attachment.ContentType != c.want {
				t.Fatalf("content type %q, want %q (%v)", attachment.ContentType, c.want, err)
			}
		})
	}
}

func TestCompleteAttachmentBeforeTheUploadIsInvalid(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	pending, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "a.txt", "text/plain", 5)
	if err != nil {
		t.Fatal(err)
	}
	before := tm.eventCount(t)

	_, err = tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID)
	wantInvalid(t, err, "attachment")

	// The attachment stays pending, so the upload can still be finished.
	tm.objects.Put(pending.ID, []byte("hello"))
	if _, err := tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID); err != nil {
		t.Fatal(err)
	}
	if tm.eventCount(t) != before+1 {
		t.Fatal("completion should write exactly one event")
	}
}

func TestCompleteAttachmentRefusesAnOversizedObjectAndDeletesIt(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	before := tm.eventCount(t)

	// A declared video that is really an image does not get the video limit.
	pending, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "clip.mp4", "video/mp4", 1000)
	if err != nil {
		t.Fatal(err)
	}
	tm.objects.PutSize(pending.ID, pngBytes(), MaxAttachmentBytes+1)

	_, err = tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID)
	wantErr(t, err, ErrTooLarge)

	if tm.objects.Has(pending.ID) {
		t.Fatal("the oversized object was kept")
	}
	var rows int
	if err := tm.store.db.QueryRow(`SELECT count(*) FROM attachments`).Scan(&rows); err != nil || rows != 0 || tm.eventCount(t) != before {
		t.Fatalf("rows %d (%v), events %d want %d", rows, err, tm.eventCount(t), before)
	}

	// A real video of the same size is fine.
	video, _, err := tm.store.BeginAttachment(tm.ctx, tm.member, issue.Key(), "clip.mp4", "video/mp4", 1000)
	if err != nil {
		t.Fatal(err)
	}
	tm.objects.PutSize(video.ID, mp4Bytes(10), MaxAttachmentBytes+1)
	done, err := tm.store.CompleteAttachment(tm.ctx, tm.member, video.ID)
	if err != nil || done.ContentType != "video/mp4" || done.ByteSize != MaxAttachmentBytes+1 {
		t.Fatalf("video %+v (%v)", done, err)
	}
}

func TestOnlyTheCreatorCanCompleteAnAttachment(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	pending := tm.beginAndUpload(t, tm.agent, issue.Key(), "a.txt", []byte("hello"))
	before := tm.eventCount(t)

	for _, other := range []Caller{tm.member, tm.owner} {
		_, err := tm.store.CompleteAttachment(tm.ctx, other, pending.ID)
		wantErr(t, err, ErrNotFound)
	}
	_, err := tm.store.CompleteAttachment(tm.ctx, tm.member, "missing")
	wantErr(t, err, ErrNotFound)

	if tm.eventCount(t) != before || len(got(tm.store.ListIssueAttachments(tm.ctx, issue.Key())).must(t)) != 0 {
		t.Fatal("a refused completion changed state")
	}
}

func TestCompletingTwiceIsIdempotent(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	pending := tm.beginAndUpload(t, tm.member, issue.Key(), "a.txt", []byte("hello"))

	first := got(tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID)).must(t)
	events := tm.eventCount(t)
	second := got(tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID)).must(t)

	if second.ID != first.ID || second.Pending || second.ContentType != first.ContentType || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("first %+v second %+v", first, second)
	}
	if tm.eventCount(t) != events {
		t.Fatal("completing again wrote another event")
	}
}

func TestPendingAttachmentsAreHidden(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	pending := tm.beginAndUpload(t, tm.member, issue.Key(), "a.txt", []byte("hello"))

	_, err := tm.store.GetAttachment(tm.ctx, pending.ID)
	wantErr(t, err, ErrNotFound)
	if list := got(tm.store.ListIssueAttachments(tm.ctx, issue.Key())).must(t); len(list) != 0 {
		t.Fatalf("list %v", list)
	}

	if _, err := tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tm.store.GetAttachment(tm.ctx, pending.ID); err != nil {
		t.Fatal(err)
	}
	if list := got(tm.store.ListIssueAttachments(tm.ctx, issue.Key())).must(t); len(list) != 1 {
		t.Fatalf("list %v", list)
	}
}

func TestDownloadURLAndReadPrefix(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	pending := tm.beginAndUpload(t, tm.member, issue.Key(), "a.txt", []byte("hello world"))
	attachment := got(tm.store.CompleteAttachment(tm.ctx, tm.member, pending.ID)).must(t)

	if url := tm.store.DownloadURL(attachment, "text/plain", "attachment"); url != "https://objects.test/get/"+attachment.ID+"?type=text/plain&disposition=attachment" {
		t.Fatalf("url %s", url)
	}

	data, more, err := tm.store.ReadAttachmentPrefix(tm.ctx, attachment, 5)
	if err != nil || string(data) != "hello" || !more {
		t.Fatalf("prefix %q more=%v (%v)", data, more, err)
	}
	data, more, err = tm.store.ReadAttachmentPrefix(tm.ctx, attachment, 11)
	if err != nil || string(data) != "hello world" || more {
		t.Fatalf("whole %q more=%v (%v)", data, more, err)
	}
}

func TestCleanFilename(t *testing.T) {
	long := strings.Repeat("é", 200) + ".png"
	cases := map[string]string{
		`C:\Users\me\shot.png`: "shot.png",
		"/etc/passwd":          "passwd",
		"a\tb\u202Ec.txt":      "abc.txt",
		"  spaced name.txt  ":  "spaced name.txt",
		"report \u00fcber.pdf": "report \u00fcber.pdf",
	}
	for raw, want := range cases {
		if got, err := cleanFilename(raw); err != nil || got != want {
			t.Errorf("cleanFilename(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}

	name, err := cleanFilename(long)
	if err != nil || len(name) > maxFilenameBytes || !strings.HasSuffix(name, ".png") || strings.ContainsRune(name, '\uFFFD') {
		t.Errorf("long name %q (%d bytes), %v", name, len(name), err)
	}
}

func TestAgentsCannotDeleteCompletedAttachments(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	pending := tm.beginAndUpload(t, tm.agent, issue.Key(), "a.txt", []byte("hello"))
	attachment := got(tm.store.CompleteAttachment(tm.ctx, tm.agent, pending.ID)).must(t)

	wantErr(t, tm.store.DeleteAttachment(tm.ctx, tm.agent, attachment.ID), ErrForbidden)
	if !tm.objects.Has(attachment.ID) {
		t.Fatal("object removed by a refused delete")
	}

	if err := tm.store.DeleteAttachment(tm.ctx, tm.member, attachment.ID); err != nil {
		t.Fatal(err)
	}
	if tm.objects.Has(attachment.ID) {
		t.Fatal("object still in the bucket")
	}
	_, err := tm.store.GetAttachment(tm.ctx, attachment.ID)
	wantErr(t, err, ErrNotFound)
	wantErr(t, tm.store.DeleteAttachment(tm.ctx, tm.member, attachment.ID), ErrNotFound)

	event := tm.lastEvent(t)
	if event.Kind != "attachment.deleted" || event.ActorID != tm.member.ActorID {
		t.Fatalf("event %+v", event)
	}
}

func TestCreatorAbandonsAPendingAttachmentWithoutAnEvent(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	pending := tm.beginAndUpload(t, tm.agent, issue.Key(), "a.txt", []byte("hello"))
	before := tm.eventCount(t)

	// Someone else cannot see it, let alone delete it.
	wantErr(t, tm.store.DeleteAttachment(tm.ctx, tm.owner, pending.ID), ErrNotFound)
	wantErr(t, tm.store.DeleteAttachment(tm.ctx, tm.member, pending.ID), ErrNotFound)
	if !tm.objects.Has(pending.ID) {
		t.Fatal("object removed by a refused delete")
	}

	if err := tm.store.DeleteAttachment(tm.ctx, tm.agent, pending.ID); err != nil {
		t.Fatal(err)
	}
	if tm.objects.Has(pending.ID) || tm.eventCount(t) != before {
		t.Fatal("abandoning left an object or wrote an event")
	}
	var rows int
	if err := tm.store.db.QueryRow(`SELECT count(*) FROM attachments`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows %d (%v)", rows, err)
	}
}

func TestDeletingAProjectRemovesItsAttachmentObjects(t *testing.T) {
	tm := newTeam(t)
	doomed := tm.issue(t, tm.project(t, "WP").ID, "Doomed")
	other := tm.project(t, "OT")
	kept := tm.issue(t, other.ID, "Kept")

	done := tm.beginAndUpload(t, tm.member, doomed.Key(), "a.txt", []byte("one"))
	got(tm.store.CompleteAttachment(tm.ctx, tm.member, done.ID)).must(t)
	pending := tm.beginAndUpload(t, tm.member, doomed.Key(), "p.txt", []byte("pending"))
	stays := tm.beginAndUpload(t, tm.member, kept.Key(), "b.txt", []byte("two"))
	got(tm.store.CompleteAttachment(tm.ctx, tm.member, stays.ID)).must(t)

	if err := tm.store.DeleteProject(tm.ctx, tm.owner, doomed.ProjectID); err != nil {
		t.Fatal(err)
	}

	if tm.objects.Has(done.ID) || tm.objects.Has(pending.ID) {
		t.Fatal("objects of the deleted project remain")
	}
	if !tm.objects.Has(stays.ID) {
		t.Fatal("object of another project removed")
	}
	list := got(tm.store.ListIssueAttachments(tm.ctx, kept.Key())).must(t)
	if len(list) != 1 || list[0].ID != stays.ID {
		t.Fatalf("rows %v", list)
	}
}

func TestAttachmentsBelongToTheirSubissueOrFinding(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	child := got(tm.store.CreateSubissue(tm.ctx, tm.agent, r.issue.ID, NewSubissue{Title: "Child", Status: IssueTodo})).must(t)
	key := r.issue.Key()

	begin := func(target AttachmentTarget, name string) Attachment {
		t.Helper()
		attachment, _, err := tm.store.BeginTargetAttachment(tm.ctx, tm.member, key, target, name, "text/plain", 5)
		if err != nil {
			t.Fatal(err)
		}
		tm.objects.Put(attachment.ID, []byte("hello"))
		return got(tm.store.CompleteAttachment(tm.ctx, tm.member, attachment.ID)).must(t)
	}
	onIssue := begin(AttachmentTarget{}, "issue.txt")
	onChild := begin(AttachmentTarget{SubissueID: child.ID}, "child.txt")
	onFinding := begin(AttachmentTarget{FindingID: r.finding.ID}, "finding.txt")

	if onChild.SubissueID != child.ID || onChild.FindingID != "" || onFinding.FindingID != r.finding.ID || onFinding.SubissueID != "" {
		t.Fatalf("targets: child %+v, finding %+v", onChild, onFinding)
	}

	names := func(list []Attachment) []string {
		out := []string{}
		for _, item := range list {
			out = append(out, item.Filename)
		}
		return out
	}
	if list := got(tm.store.ListIssueAttachments(tm.ctx, key)).must(t); len(list) != 1 || list[0].ID != onIssue.ID {
		t.Errorf("issue list %v, want only issue.txt", names(list))
	}
	if list := got(tm.store.ListTargetAttachments(tm.ctx, key, AttachmentTarget{SubissueID: child.ID})).must(t); len(list) != 1 || list[0].ID != onChild.ID {
		t.Errorf("subissue list %v, want only child.txt", names(list))
	}
	if list := got(tm.store.ListTargetAttachments(tm.ctx, key, AttachmentTarget{FindingID: r.finding.ID})).must(t); len(list) != 1 || list[0].ID != onFinding.ID {
		t.Errorf("finding list %v, want only finding.txt", names(list))
	}
}

func TestAttachmentTargetsMustBeOnTheSameIssue(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	other := tm.issue(t, r.issue.ProjectID, "Other")
	otherChild := got(tm.store.CreateSubissue(tm.ctx, tm.agent, other.ID, NewSubissue{Title: "Child", Status: IssueTodo})).must(t)
	child := got(tm.store.CreateSubissue(tm.ctx, tm.agent, r.issue.ID, NewSubissue{Title: "Child", Status: IssueTodo})).must(t)

	cases := map[string]AttachmentTarget{
		"subissue of another issue": {SubissueID: otherChild.ID},
		"finding of another issue":  {FindingID: r.finding.ID},
		"unknown subissue":          {SubissueID: "missing"},
		"both targets":              {SubissueID: child.ID, FindingID: r.finding.ID},
	}
	for name, target := range cases {
		issueKey := r.issue.Key()
		if name == "finding of another issue" {
			issueKey = other.Key()
		}
		before := tm.eventCount(t)
		if _, _, err := tm.store.BeginTargetAttachment(tm.ctx, tm.member, issueKey, target, "a.txt", "text/plain", 5); err == nil {
			t.Errorf("%s: attachment was started", name)
		}
		if tm.eventCount(t) != before {
			t.Errorf("%s: an event was recorded", name)
		}
	}
}
