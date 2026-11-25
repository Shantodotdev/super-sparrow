package discord

import (
	"slices"
	"strings"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func (h *discordHarness) pullRequest(t *testing.T, project string) store.GitHubPullRequest {
	t.Helper()
	return store.GitHubPullRequest{
		Repository: "acme/shop", Number: 12, Title: "Fix *checkout* flow", URL: "https://github.com/acme/shop/pull/12",
		AuthorLogin: "Octocat", HeadRef: "fix/checkout", BaseRef: "main", Additions: 10, Deletions: 2,
		RequestedReviewers: []string{"hubot", "stranger_1"}, MergedByLogin: "hubot",
		ProjectID: must(t, wrap(h.st.GetProjectByKey(t.Context(), project))).ID,
	}
}

// route links Sam and Rae to GitHub logins and sends OPS pull request news to the news channel.
func (h *discordHarness) route(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	h.setup(t, "n1", firstUser, h.first.ID, "")
	h.setup(t, "n2", secondUser, h.second.ID, "")
	must(t, wrap(h.st.SetGitHubLogin(ctx, store.Caller{ActorID: h.first.ID}, h.first.ID, "octocat")))
	must(t, wrap(h.st.SetGitHubLogin(ctx, store.Caller{ActorID: h.second.ID}, h.second.ID, "hubot")))
	must(t, wrap(h.st.SetDiscordProjectChannel(ctx, h.bot.guildSource(guildID, firstUser), "OPS", newsChannel)))
}

func (h *discordHarness) noPost(t *testing.T) {
	t.Helper()
	select {
	case post := <-h.posts:
		t.Fatalf("unexpected Discord post: %+v", post)
	default:
	}
}

func TestPullRequestOpenedPostsToProjectChannel(t *testing.T) {
	h := newDiscordHarness(t)
	h.route(t)

	h.bot.PullRequestChanged(t.Context(), store.GitHubPullRequestChange{PullRequest: h.pullRequest(t, "OPS"), Kind: "github.pull_request.opened"})
	post := <-h.posts
	want := "**New pull request · OPS**\n" +
		"[acme/shop#12 Fix \\*checkout\\* flow](https://github.com/acme/shop/pull/12)\n" +
		"fix/checkout → main · +10 −2\n" +
		"by <@" + firstUser + ">\n" +
		"Review requested from <@" + secondUser + ">, stranger\\_1"
	if post.Content != want {
		t.Fatalf("message:\n%s\nwant:\n%s", post.Content, want)
	}
	if post.Flags != 4 || post.AllowedMentions.Parse == nil || len(post.AllowedMentions.Parse) != 0 ||
		!slices.Equal(post.AllowedMentions.Users, []string{firstUser, secondUser}) {
		t.Fatalf("unexpected flags or allowed mentions: %+v", post)
	}

	pull := h.pullRequest(t, "OPS")
	pull.Draft = false
	h.bot.PullRequestChanged(t.Context(), store.GitHubPullRequestChange{PullRequest: pull, Kind: "github.pull_request.ready_for_review"})
	if post := <-h.posts; !strings.HasPrefix(post.Content, "**New pull request · OPS**") {
		t.Fatalf("ready for review was not announced: %+v", post)
	}
}

func TestPullRequestMergedMentionsAuthorAndMerger(t *testing.T) {
	h := newDiscordHarness(t)
	h.route(t)

	h.bot.PullRequestChanged(t.Context(), store.GitHubPullRequestChange{PullRequest: h.pullRequest(t, "OPS"), Kind: "github.pull_request.merged"})
	post := <-h.posts
	want := "**Pull request merged · OPS**\n" +
		"[acme/shop#12 Fix \\*checkout\\* flow](https://github.com/acme/shop/pull/12)\n" +
		"<@" + firstUser + ">, your PR was merged by <@" + secondUser + ">."
	if post.Content != want || post.Flags != 4 || !slices.Equal(post.AllowedMentions.Users, []string{firstUser, secondUser}) || len(post.AllowedMentions.Parse) != 0 {
		t.Fatalf("unexpected merged message: %+v", post)
	}

	unmappedMerger := h.pullRequest(t, "OPS")
	unmappedMerger.MergedByLogin = "stranger_1"
	h.bot.PullRequestChanged(t.Context(), store.GitHubPullRequestChange{PullRequest: unmappedMerger, Kind: "github.pull_request.merged"})
	post = <-h.posts
	wantUnmapped := "**Pull request merged · OPS**\n" +
		"[acme/shop#12 Fix \\*checkout\\* flow](https://github.com/acme/shop/pull/12)\n" +
		"<@" + firstUser + ">, your PR was merged by stranger\\_1."
	if post.Content != wantUnmapped || !slices.Equal(post.AllowedMentions.Users, []string{firstUser}) {
		t.Fatalf("unexpected merged message with unmapped merger: %+v", post)
	}

	pull := h.pullRequest(t, "OPS")
	pull.MergedByLogin = "octocat"
	h.bot.PullRequestChanged(t.Context(), store.GitHubPullRequestChange{PullRequest: pull, Kind: "github.pull_request.merged"})
	post = <-h.posts
	wantSelf := "**Pull request merged · OPS**\n" +
		"[acme/shop#12 Fix \\*checkout\\* flow](https://github.com/acme/shop/pull/12)\n" +
		"<@" + firstUser + ">, your PR was merged."
	if post.Content != wantSelf || !slices.Equal(post.AllowedMentions.Users, []string{firstUser}) {
		t.Fatalf("the author who merged was mentioned twice: %+v", post)
	}
}

func TestPullRequestSkipsUnwantedChanges(t *testing.T) {
	h := newDiscordHarness(t)
	h.route(t)
	ctx := t.Context()

	draft := h.pullRequest(t, "OPS")
	draft.Draft = true
	h.bot.PullRequestChanged(ctx, store.GitHubPullRequestChange{PullRequest: draft, Kind: "github.pull_request.opened"})
	h.bot.PullRequestChanged(ctx, store.GitHubPullRequestChange{PullRequest: h.pullRequest(t, "OPS"), Kind: "github.pull_request.closed"})

	unrouted := h.pullRequest(t, "WP")
	h.bot.PullRequestChanged(ctx, store.GitHubPullRequestChange{PullRequest: unrouted, Kind: "github.pull_request.opened"})

	standalone := h.pullRequest(t, "OPS")
	standalone.ProjectID = ""
	h.bot.PullRequestChanged(ctx, store.GitHubPullRequestChange{PullRequest: standalone, Kind: "github.pull_request.opened"})
	h.noPost(t)
}

func TestPullRequestSkipsUnapprovedServer(t *testing.T) {
	h := newDiscordHarness(t)
	ctx := t.Context()
	owner := store.Caller{ActorID: h.first.ID}
	h.bot.observeGuild(ctx, otherGuild, "Sandbox")
	must(t, wrap(h.bot.SetGuildApproval(ctx, owner, otherGuild, store.DiscordGuildApproved)))
	must(t, wrap(h.st.SetDiscordProjectChannel(ctx, h.bot.guildSource(otherGuild, firstUser), "OPS", newsChannel)))

	h.bot.PullRequestChanged(ctx, store.GitHubPullRequestChange{PullRequest: h.pullRequest(t, "OPS"), Kind: "github.pull_request.opened"})
	<-h.posts

	must(t, wrap(h.bot.SetGuildApproval(ctx, owner, otherGuild, store.DiscordGuildRevoked)))
	h.bot.PullRequestChanged(ctx, store.GitHubPullRequestChange{PullRequest: h.pullRequest(t, "OPS"), Kind: "github.pull_request.opened"})
	h.noPost(t)
}

func TestPullRequestMessageShortensTitleAndKeepsUnmappedPlain(t *testing.T) {
	announcement := pullRequestMessage("github.pull_request.opened", "OPS", store.GitHubPullRequest{
		Repository: "acme/shop", Number: 1, Title: strings.Repeat("long ", 80), URL: "https://github.com/acme/shop/pull/1",
		AuthorLogin: "ghost",
	}, nil)
	if len([]rune(announcement.Content)) > 400 || len(announcement.AllowedMentions.Users) != 0 || !strings.Contains(announcement.Content, "by ghost") {
		t.Fatalf("long titles must be shortened and unmapped logins stay plain: %q", announcement.Content)
	}
}

func TestDiscordSetGitHubChannelAndLinkGitHub(t *testing.T) {
	h := newDiscordHarness(t)
	ctx := t.Context()

	// An unlinked member may point a project at the channel they are in.
	h.ask(t, setupUser, "80", "Post GitHub updates for OPS here")
	if reply := h.replied(t); reply.Content != "I’ll post OPS pull request updates in this channel." {
		t.Fatalf("unexpected channel reply: %+v", reply)
	}
	project := must(t, wrap(h.st.GetProjectByKey(ctx, "OPS")))
	channel := must(t, wrap(h.st.DiscordChannelForProject(ctx, project.ID)))
	if channel.ChannelID != channelID || channel.GuildID != guildID {
		t.Fatalf("channel was not saved from the mention: %+v", channel)
	}

	// Linking GitHub needs a linked teammate and always targets the requester's own person.
	h.ask(t, setupUser, "81", "My GitHub is octocat")
	if reply := h.replied(t); !strings.Contains(reply.Content, "Link your Discord account") {
		t.Fatalf("an unlinked member could link GitHub: %+v", reply)
	}
	h.setup(t, "82", firstUser, h.second.ID, "")
	h.ask(t, firstUser, "83", "My GitHub is octocat")
	if reply := h.replied(t); reply.Content != "Linked your Sparrow account to GitHub @octocat." {
		t.Fatalf("unexpected GitHub link reply: %+v", reply)
	}
	accounts := must(t, wrap(h.st.ListGitHubAccounts(ctx)))
	if len(accounts) != 1 || accounts[0].PersonID != h.second.ID || accounts[0].Login != "octocat" {
		t.Fatalf("login was saved for the wrong teammate: %+v", accounts)
	}

	h.ask(t, firstUser, "84", "My GitHub is not valid")
	if reply := h.replied(t); strings.Contains(reply.Content, "Linked") || strings.Contains(reply.Content, "couldn’t complete") {
		t.Fatalf("an invalid login was not explained: %+v", reply)
	}
}
