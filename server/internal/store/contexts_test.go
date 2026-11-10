package store

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// pngBytes returns a small valid PNG image.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestContextsAreNumberedPerProjectAndRecordTheirActor(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	other := tm.project(t, "SS")
	issue := tm.issue(t, project.ID, "Checkout")

	first := got(tm.store.CreateContext(tm.ctx, tm.member, NewContext{ProjectID: project.ID, Title: " Competitor flow ", Body: "Notes"})).must(t)
	second := got(tm.store.CreateContext(tm.ctx, tm.agent, NewContext{ProjectID: project.ID, Title: "Error copy", IssueID: issue.ID})).must(t)
	elsewhere := got(tm.store.CreateContext(tm.ctx, tm.member, NewContext{ProjectID: other.ID, Title: "Other"})).must(t)

	if first.Key() != "WP-C1" || second.Key() != "WP-C2" || elsewhere.Key() != "SS-C1" {
		t.Fatalf("keys %s %s %s", first.Key(), second.Key(), elsewhere.Key())
	}
	if first.Title != "Competitor flow" || first.BodyRevision != 1 || first.CreatedByID != tm.member.ActorID {
		t.Fatalf("first %+v", first)
	}
	if second.IssueKey != "WP-1" {
		t.Fatalf("linked issue key %q", second.IssueKey)
	}
	if event := tm.lastEvent(t); event.Kind != "context.created" || event.ProjectID != other.ID {
		t.Fatalf("event %+v", event)
	}

	found := got(tm.store.GetContextByKey(tm.ctx, "wp-c2")).must(t)
	if found.ID != second.ID {
		t.Fatalf("found %+v", found)
	}
	if _, err := tm.store.GetContextByKey(tm.ctx, "WP-2"); err == nil {
		t.Fatal("an issue key was read as a context key")
	}

	listed := got(tm.store.ListContexts(tm.ctx, ContextFilter{ProjectID: project.ID})).must(t)
	if len(listed) != 2 || listed[0].ID != second.ID {
		t.Fatalf("listed %+v", listed)
	}
	linked := got(tm.store.ListContexts(tm.ctx, ContextFilter{ProjectID: project.ID, IssueID: issue.ID})).must(t)
	if len(linked) != 1 || linked[0].ID != second.ID {
		t.Fatalf("linked %+v", linked)
	}
	searched := got(tm.store.ListContexts(tm.ctx, ContextFilter{ProjectID: project.ID, Search: "COMPETITOR"})).must(t)
	if len(searched) != 1 || searched[0].ID != first.ID {
		t.Fatalf("searched %+v", searched)
	}
}

func TestContextRefusesIssuesFromAnotherProject(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	other := tm.project(t, "SS")
	foreign := tm.issue(t, other.ID, "Foreign")

	_, err := tm.store.CreateContext(tm.ctx, tm.member, NewContext{ProjectID: project.ID, Title: "Notes", IssueID: foreign.ID})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "issue_id" {
		t.Fatalf("err %v", err)
	}

	created := got(tm.store.CreateContext(tm.ctx, tm.member, NewContext{ProjectID: project.ID, Title: "Notes"})).must(t)
	_, err = tm.store.UpdateContext(tm.ctx, tm.member, created.ID, ContextUpdate{IssueID: Change(foreign.ID)})
	if !errors.As(err, &validation) || validation.Field != "issue_id" {
		t.Fatalf("update err %v", err)
	}
}

func TestContextBodyRevisionsRefuseStaleEdits(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	created := got(tm.store.CreateContext(tm.ctx, tm.member, NewContext{ProjectID: project.ID, Title: "Notes", Body: "One"})).must(t)

	updated := got(tm.store.UpdateContext(tm.ctx, tm.agent, created.ID, ContextUpdate{Body: Change("Two"), BodyRevision: 1})).must(t)
	if updated.Body != "Two" || updated.BodyRevision != 2 {
		t.Fatalf("updated %+v", updated)
	}

	_, err := tm.store.UpdateContext(tm.ctx, tm.member, created.ID, ContextUpdate{Body: Change("Stale"), BodyRevision: 1})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit err %v", err)
	}
	if _, err := tm.store.UpdateContext(tm.ctx, tm.member, created.ID, ContextUpdate{Body: Change("Missing revision")}); err == nil {
		t.Fatal("a body edit without its revision was accepted")
	}

	before := tm.eventCount(t)
	unchanged := got(tm.store.UpdateContext(tm.ctx, tm.member, created.ID, ContextUpdate{Title: Change("Notes")})).must(t)
	if unchanged.BodyRevision != 2 || tm.eventCount(t) != before {
		t.Fatal("an update that changed nothing wrote an event")
	}
}

func TestOnlyPeopleDeleteContexts(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	created := got(tm.store.CreateContext(tm.ctx, tm.member, NewContext{ProjectID: project.ID, Title: "Notes"})).must(t)

	if err := tm.store.DeleteContext(tm.ctx, tm.agent, created.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("agent delete err %v", err)
	}
	if err := tm.store.DeleteContext(tm.ctx, tm.member, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tm.store.GetContextByKey(tm.ctx, "WP-C1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted context err %v", err)
	}
	if event := tm.lastEvent(t); event.Kind != "context.deleted" {
		t.Fatalf("event %s", event.Kind)
	}
}

func TestAttachmentsAcceptOnlyImages(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")

	for name, data := range map[string][]byte{
		"empty": {},
		"svg":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"html":  []byte(`<html><body>hi</body></html>`),
		"large": append(pngBytes(t), make([]byte, MaxAttachmentBytes)...),
	} {
		var validation *ValidationError
		if _, err := tm.store.CreateAttachment(tm.ctx, tm.member, project.ID, data); !errors.As(err, &validation) {
			t.Fatalf("%s: err %v", name, err)
		}
	}

	data := pngBytes(t)
	attachment := got(tm.store.CreateAttachment(tm.ctx, tm.agent, project.ID, data)).must(t)
	if attachment.ContentType != "image/png" || attachment.ByteSize != len(data) || attachment.CreatedByID != tm.agent.ActorID {
		t.Fatalf("attachment %+v", attachment)
	}
	if event := tm.lastEvent(t); event.Kind != "attachment.created" || event.ActorID != tm.agent.ActorID {
		t.Fatalf("event %+v", event)
	}
	stored := got(tm.store.GetAttachment(tm.ctx, attachment.ID)).must(t)
	if !bytes.Equal(stored.Data, data) {
		t.Fatal("stored bytes differ")
	}
}

func TestReferencedAttachmentsStayInTheirProject(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	other := tm.project(t, "SS")
	first := got(tm.store.CreateAttachment(tm.ctx, tm.member, project.ID, pngBytes(t))).must(t)
	second := got(tm.store.CreateAttachment(tm.ctx, tm.member, project.ID, pngBytes(t))).must(t)
	foreign := got(tm.store.CreateAttachment(tm.ctx, tm.member, other.ID, pngBytes(t))).must(t)

	markdown := "![two](" + second.URLPath() + ")\n![one](https://sparrow.example.com" + first.URLPath() + ")\n" +
		"![again](" + second.URLPath() + ")\n![foreign](" + foreign.URLPath() + ")\n" +
		"![missing](/api/attachments/01900000-0000-7000-8000-000000000000)"
	found := got(tm.store.ReferencedAttachments(tm.ctx, project.ID, markdown)).must(t)
	if len(found) != 2 || found[0].ID != second.ID || found[1].ID != first.ID {
		t.Fatalf("found %+v", found)
	}
}
