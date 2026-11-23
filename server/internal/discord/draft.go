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

// setupDraft is a member link or a default project change waiting for the requester's button.
// Exactly one of Member and ProjectKey is set.
type setupDraft struct {
	Member      store.DiscordMember
	PersonID    string
	PersonName  string
	ProjectKey  string
	ProjectName string
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
	return h.offerDraft(interactionID, draft{caller: caller, issue: &fields}), nil
}

// offerDraft saves a draft and shows it with its confirmation button.
func (h *Bot) offerDraft(interactionID string, proposal draft) message {
	saved, accepted := h.saveDraft(interactionID, proposal)
	if !accepted {
		return plain("Sparrow has too many pending drafts. Try again in a few minutes.")
	}
	if saved.caller != proposal.caller {
		return plain("Your teammate mapping changed. Mention me again.")
	}

	var content, label, prefix string
	switch {
	case saved.issue != nil:
		content, label, prefix = draftPreview(*saved.issue), "Create issue", confirmPrefix
	case saved.setup != nil && saved.setup.ProjectKey != "":
		content = fmt.Sprintf("**Use %s (%s) as this server’s default project?**", escape(saved.setup.ProjectName), saved.setup.ProjectKey)
		label, prefix = "Set default project", setupPrefix
	case saved.setup != nil:
		content = fmt.Sprintf("**Link Discord member %s to Sparrow teammate %s?**", escape(saved.setup.Member.Name), escape(saved.setup.PersonName))
		label, prefix = "Link member", setupPrefix
	}
	if len([]rune(content)) > 1850 {
		return plain("The draft is too long to preview in Discord. Try again with a shorter description.")
	}
	reply := plain(content)
	reply.Components = &[]actionRow{{Type: 1, Components: []button{{
		Type: 2, Style: 1, Label: label, CustomID: prefix + interactionID,
	}}}}
	return reply
}

func draftPreview(fields issueDraft) string {
	content := fmt.Sprintf("**Create an issue in %s?**\n%s · %s", fields.ProjectKey, escape(fields.Title), statusLabel(fields.Status))
	if fields.Plan != "" {
		content += "\n\n" + escape(fields.Plan)
	}
	return content
}

func (h *Bot) saveDraft(id string, proposal draft) (draft, bool) {
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

	proposal.expires = now.Add(draftLifetime)
	h.drafts[id] = proposal
	return proposal, true
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

// confirm applies a draft for the Discord user who requested it. The issue uses the existing tool
// receipt and setup changes use the message's setup receipt, so retrying a button never applies
// a change twice.
func (h *Bot) confirm(ctx context.Context, source store.DiscordIdentity, customID string) response {
	id, isIssue := strings.CutPrefix(customID, confirmPrefix)
	if !isIssue {
		var isSetup bool
		if id, isSetup = strings.CutPrefix(customID, setupPrefix); !isSetup {
			return private("That Sparrow action is unavailable.")
		}
	}
	h.draftMu.Lock()
	saved, present := h.drafts[id]
	h.draftMu.Unlock()
	if !present || !h.now().Before(saved.expires) || saved.caller.Discord != source ||
		(isIssue && saved.issue == nil) || (!isIssue && saved.setup == nil) {
		return private("This draft is unavailable. Mention me to make a new one.")
	}

	var content string
	if isIssue {
		var failure *response
		content, failure = h.createDraftIssue(ctx, saved.caller, id, *saved.issue)
		if failure != nil {
			return *failure
		}
	} else if saved.setup.ProjectKey != "" {
		project, err := h.store.SetDiscordDefaultProject(ctx, saved.caller, id, saved.setup.ProjectKey)
		if err != nil {
			return private(h.userError(ctx, "set Discord default project", err))
		}
		content = "I’ll use **" + escape(project.Name) + " (" + project.Key + ")** when you leave out the project."
	} else {
		person, err := h.store.LinkDiscordMember(ctx, saved.caller, id, saved.setup.Member, saved.setup.PersonID)
		if err != nil {
			return private(h.userError(ctx, "link Discord member", err))
		}
		content = "Linked " + escape(saved.setup.Member.Name) + " to **" + escape(person.DisplayName) + "** in Sparrow."
	}

	reply := plain(content)
	reply.Components = &[]actionRow{}
	return response{Type: 7, Data: reply}
}

func (h *Bot) createDraftIssue(ctx context.Context, caller store.Caller, id string, fields issueDraft) (string, *response) {
	arguments := struct {
		issueDraft
		RetryKey string `json:"idempotency_key"`
	}{fields, "discord:" + id}
	result, failed, err := h.tools.Call(ctx, caller, "create_issue", arguments)
	if err != nil {
		reply := private(h.userError(ctx, "create Discord issue", err))
		return "", &reply
	}
	if failed {
		reply := private(toolFailure(result))
		return "", &reply
	}

	var issue issueRow
	if err := json.Unmarshal(result, &issue); err != nil {
		reply := private(h.userError(ctx, "read created Discord issue", err))
		return "", &reply
	}
	return fmt.Sprintf("Created **%s** · %s · %s", issue.Key, escape(shorten(oneLine(issue.Title), 200)), statusLabel(issue.Status)), nil
}
