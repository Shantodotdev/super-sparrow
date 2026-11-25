package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	suppressEmbeds   = 4
	maxPullTitle     = 100
	pullOpened       = "github.pull_request.opened"
	pullReadyForView = "github.pull_request.ready_for_review"
	pullMerged       = "github.pull_request.merged"
)

// PullRequestChanged posts a pull request's news to its project's Discord channel. It does
// nothing for changes that are not worth a message, for projects without a channel, and when
// the channel's server is no longer approved.
func (h *Bot) PullRequestChanged(ctx context.Context, change store.GitHubPullRequestChange) {
	pull := change.PullRequest
	switch change.Kind {
	case pullOpened:
		if pull.Draft {
			return
		}
	case pullReadyForView, pullMerged:
	default:
		return
	}
	if pull.ProjectID == "" {
		return
	}

	channel, err := h.store.DiscordChannelForProject(ctx, pull.ProjectID)
	if errors.Is(err, store.ErrNotFound) {
		return
	}
	if err != nil {
		h.logger.WarnContext(ctx, "find Discord channel for pull request", "repository", pull.Repository, "number", pull.Number, "error", err)
		return
	}
	status, err := h.guildAccess(ctx, h.guildSource(channel.GuildID, ""))
	if err != nil {
		h.logger.WarnContext(ctx, "check Discord server approval for pull request", "repository", pull.Repository, "number", pull.Number, "error", err)
		return
	}
	if status != store.DiscordGuildApproved {
		return
	}

	logins := append([]string{pull.AuthorLogin, pull.MergedByLogin}, pull.RequestedReviewers...)
	users, err := h.store.DiscordUsersForGitHubLogins(ctx, channel.GuildID, logins)
	if err != nil {
		h.logger.WarnContext(ctx, "map GitHub logins to Discord users", "repository", pull.Repository, "number", pull.Number, "error", err)
		return
	}

	announcement := pullRequestMessage(change.Kind, channel.ProjectKey, pull, users)
	body, err := json.Marshal(announcement)
	if err != nil {
		h.logger.WarnContext(ctx, "encode Discord pull request message", "repository", pull.Repository, "number", pull.Number, "error", err)
		return
	}
	if err := h.postChannelMessage(ctx, channel.ChannelID, body); err != nil {
		h.logger.WarnContext(ctx, "post Discord pull request message", "repository", pull.Repository, "number", pull.Number, "error", err)
	}
}

// pullRequestMessage renders a pull request announcement. Only the people it names through users
// can be pinged; everyone else appears as plain text.
func pullRequestMessage(kind, projectKey string, pull store.GitHubPullRequest, users map[string]string) message {
	var pinged []string
	person := func(login string) string {
		id, ok := users[strings.ToLower(login)]
		if !ok {
			return escape(login)
		}
		if !slices.Contains(pinged, id) {
			pinged = append(pinged, id)
		}
		return "<@" + id + ">"
	}

	link := fmt.Sprintf("[%s](%s)",
		escape(fmt.Sprintf("%s#%d %s", pull.Repository, pull.Number, shorten(oneLine(pull.Title), maxPullTitle))),
		strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(pull.URL))

	var lines []string
	if kind == pullMerged {
		lines = []string{"**Pull request merged · " + escape(projectKey) + "**", link}
		merger := pull.MergedByLogin
		switch {
		case pull.AuthorLogin != "" && merger != "" && !strings.EqualFold(merger, pull.AuthorLogin):
			lines = append(lines, person(pull.AuthorLogin)+", your PR was merged by "+person(merger)+".")
		case pull.AuthorLogin != "":
			lines = append(lines, person(pull.AuthorLogin)+", your PR was merged.")
		case merger != "":
			lines = append(lines, "Merged by "+person(merger)+".")
		}
	} else {
		lines = []string{"**New pull request · " + escape(projectKey) + "**", link,
			fmt.Sprintf("%s → %s · +%d −%d", escape(oneLine(pull.HeadRef)), escape(oneLine(pull.BaseRef)), pull.Additions, pull.Deletions),
			"by " + person(pull.AuthorLogin)}
		if len(pull.RequestedReviewers) > 0 {
			reviewers := make([]string, 0, len(pull.RequestedReviewers))
			for _, login := range pull.RequestedReviewers {
				reviewers = append(reviewers, person(login))
			}
			lines = append(lines, "Review requested from "+strings.Join(reviewers, ", "))
		}
	}

	return message{
		Content:         shorten(strings.Join(lines, "\n"), 1850),
		Flags:           suppressEmbeds,
		AllowedMentions: allowed{Parse: []string{}, Users: pinged},
	}
}
