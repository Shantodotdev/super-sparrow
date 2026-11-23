package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// todayStatus uses the server's configured team time zone and Sparrow's committed issue data.
func (h *Bot) todayStatus(ctx context.Context, projectKey string) (message, error) {
	project, err := h.store.GetProjectByKey(ctx, projectKey)
	if err != nil {
		return message{}, err
	}
	now := h.now().In(h.cfg.TimeZone)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.cfg.TimeZone)
	counts, changes, err := h.store.DiscordProjectStatus(ctx, project.ID, start, start.AddDate(0, 0, 1))
	if err != nil {
		return message{}, err
	}
	lines := []string{fmt.Sprintf("**%s today · %s**", escape(shorten(project.Name, 90)), now.Format("Jan 2"))}
	var totals []string
	for _, status := range []store.IssueStatus{
		store.IssueBacklog, store.IssueTodo, store.IssueInProgress, store.IssueInReview, store.IssueDone, store.IssueCanceled,
	} {
		if count := counts[status]; count > 0 {
			totals = append(totals, fmt.Sprintf("%d %s", count, statusLabel(string(status))))
		}
	}
	if len(totals) == 0 {
		return plain(strings.Join(append(lines, "This project has no issues yet."), "\n")), nil
	}
	lines = append(lines, "Current issues: "+strings.Join(totals, " · "))
	if len(changes) == 0 {
		return plain(strings.Join(append(lines, "No issues were opened or moved today."), "\n")), nil
	}
	lines = append(lines, "", "**Today’s issue changes**")
	for _, change := range changes[:min(len(changes), 10)] {
		action := "moved to "
		if change.New {
			action = "opened as "
		}
		lines = append(lines, fmt.Sprintf("• **%s** · %s · %s%s", change.Key, escape(shorten(oneLine(change.Title), 60)), action, statusLabel(string(change.Status))))
	}
	if len(changes) > 10 {
		lines = append(lines, "Showing the latest 10 changes.")
	}
	return plain(strings.Join(lines, "\n")), nil
}
