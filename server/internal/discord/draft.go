package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

type issueDraft struct {
	ProjectKey string `json:"project_key"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Plan       string `json:"plan"`
}

// prepareDraft validates and displays a proposal. The immutable arguments are held until the same
// linked Discord user confirms. Repeated deliveries of the original command keep the first draft.
func (h *Bot) prepareDraft(ctx context.Context, caller store.Caller, interactionID string, arguments map[string]json.RawMessage) (message, error) {
	fields, err := decodeDraft(arguments)
	if err != nil {
		return plain("The draft needs a valid title, starting status, and project. Mention me again."), nil
	}
	project, err := h.store.GetProjectByKey(ctx, fields.ProjectKey)
	if err != nil {
		return message{}, err
	}
	fields.ProjectKey = project.Key

	if len([]rune(draftPreview(fields))) > 1850 {
		return plain("The draft is too long to preview in Discord. Try again with a shorter description."), nil
	}

	saved, accepted := h.saveDraft(interactionID, caller, fields)
	if !accepted {
		return plain("Sparrow has too many pending drafts. Try again in a few minutes."), nil
	}
	if saved.caller != caller {
		return plain("Your teammate mapping changed. Mention me again."), nil
	}

	content := draftPreview(saved.fields)
	if len([]rune(content)) > 1850 {
		return plain("The draft is too long to preview in Discord. Try again with a shorter description."), nil
	}
	reply := plain(content)
	reply.Components = &[]actionRow{{Type: 1, Components: []button{{
		Type: 2, Style: 1, Label: "Create issue", CustomID: confirmPrefix + interactionID,
	}}}}
	return reply, nil
}

func draftPreview(fields issueDraft) string {
	content := fmt.Sprintf("**Create an issue in %s?**\n%s · %s", fields.ProjectKey, escape(fields.Title), statusLabel(fields.Status))
	if fields.Plan != "" {
		content += "\n\n" + escape(fields.Plan)
	}
	return content
}

func (h *Bot) saveDraft(id string, caller store.Caller, fields issueDraft) (draft, bool) {
	h.draftMu.Lock()
	defer h.draftMu.Unlock()

	now := h.now()
	for id, saved := range h.drafts {
		if !now.Before(saved.expires) {
			delete(h.drafts, id)
		}
	}
	if saved, exists := h.drafts[id]; exists {
		return saved, true
	}
	if len(h.drafts) >= 256 {
		return draft{}, false
	}

	saved := draft{caller: caller, expires: now.Add(draftLifetime), fields: fields}
	h.drafts[id] = saved
	return saved, true
}

func decodeDraft(arguments map[string]json.RawMessage) (issueDraft, error) {
	var fields issueDraft
	raw, err := json.Marshal(arguments)
	if err != nil {
		return fields, fmt.Errorf("encode issue draft fields: %w", err)
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fields, fmt.Errorf("decode issue draft fields: %w", err)
	}
	fields.Title, fields.Plan = oneLine(fields.Title), strings.TrimSpace(fields.Plan)
	if err := store.ValidateNewIssue(store.NewIssue{Title: fields.Title, Plan: fields.Plan, Status: store.IssueStatus(fields.Status)}); err != nil {
		return fields, err
	}
	return fields, nil
}

// confirm uses the existing tool receipt so retrying a button can never allocate another issue.
func (h *Bot) confirm(ctx context.Context, source store.DiscordIdentity, customID string) response {
	id, ok := strings.CutPrefix(customID, confirmPrefix)
	if !ok {
		return private("That Sparrow action is unavailable.")
	}
	h.draftMu.Lock()
	saved, present := h.drafts[id]
	h.draftMu.Unlock()
	if !present || !h.now().Before(saved.expires) || saved.caller.Discord != source {
		return private("This draft is unavailable. Mention me to make a new one.")
	}

	arguments := struct {
		issueDraft
		RetryKey string `json:"idempotency_key"`
	}{saved.fields, "discord:" + id}
	result, failed, err := h.tools.Call(ctx, saved.caller, "create_issue", arguments)
	if err != nil {
		return private(h.userError(ctx, "create Discord issue", err))
	}
	if failed {
		return private(toolFailure(result))
	}

	var issue issueRow
	if err := json.Unmarshal(result, &issue); err != nil {
		return private(h.userError(ctx, "read created Discord issue", err))
	}
	reply := plain(fmt.Sprintf("Created **%s** · %s · %s", issue.Key, escape(shorten(oneLine(issue.Title), 200)), statusLabel(issue.Status)))
	reply.Components = &[]actionRow{}
	return response{Type: 7, Data: reply}
}
