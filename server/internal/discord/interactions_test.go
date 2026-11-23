package discord

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/shantodotdev/super-sparrow/server/internal/config"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/mcp"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

const (
	appID      = "1234567890"
	channelID  = "9876543210"
	guildID    = "2345678901"
	otherGuild = "2345678902"
	thirdGuild = "2345678903"
	firstUser  = "3456789012"
	secondUser = "4567890123"
	setupUser  = "5678901234"
	testModel  = "claude-haiku-5-5"
)

type discordHarness struct {
	*httptest.Server
	bot        *Bot
	privateKey ed25519.PrivateKey
	now        time.Time
	st         *store.Store
	first      store.Actor
	second     store.Actor
	modelCalls atomic.Int32
	replies    chan message
	modelGate  chan struct{}
}

func newDiscordHarness(t *testing.T, extraGuilds ...string) *discordHarness {
	t.Helper()
	ctx := t.Context()
	db := must(t, wrap(database.Open(ctx, filepath.Join(t.TempDir(), "discord.db"))))
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &discordHarness{privateKey: private, now: time.Now().Truncate(time.Second), replies: make(chan message, 10), modelGate: make(chan struct{}, 1)}
	clock := func() time.Time { return h.now }
	h.st = store.New(db, clock, nil)
	system := store.Caller{ActorID: store.SystemActorID}
	h.first = must(t, wrap(h.st.CreateHuman(ctx, system, store.NewHuman{DisplayName: "Sam", Email: "sam@example.com", Role: store.RoleOwner})))
	h.second = must(t, wrap(h.st.CreateHuman(ctx, system, store.NewHuman{DisplayName: "Rae", Email: "rae@example.com", Role: store.RoleMember})))
	owner := store.Caller{ActorID: h.first.ID}
	must(t, wrap(h.st.CreateProject(ctx, owner, store.NewProject{Key: "WP", Name: "Checkout"})))
	must(t, wrap(h.st.CreateProject(ctx, owner, store.NewProject{Key: "OPS", Name: "Operations"})))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publicURL := must(t, wrap(url.Parse("https://sparrow.example")))
	tools := mcp.New(h.st, publicURL, logger, "test")
	endpoint := New(config.Discord{ApplicationID: appID, GuildIDs: append([]string{guildID}, extraGuilds...), PublicKey: public, Model: testModel, BotToken: "test-token", TimeZone: time.UTC}, h.st, tools, logger, clock)
	anthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h.modelCalls.Add(1)
		if req.Method != http.MethodPost || req.Header.Get("x-api-key") != "test-key" || req.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("bad Anthropic request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input struct {
			Model    string           `json:"model"`
			Messages []conversation   `json:"messages"`
			Tools    []mcp.Definition `json:"tools"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil || input.Model != testModel || len(input.Messages) == 0 || (len(input.Tools) != 3 && len(input.Tools) != 6) {
			t.Errorf("bad model request: %+v, %v", input, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, tool := range input.Tools {
			switch tool.Name {
			case "create_issue", "list_projects", "list_issues", "link_teammate", "set_default_project", "today_status":
			default:
				t.Errorf("unexpected model tool: %s", tool.Name)
			}
		}

		var result string
		switch input.Messages[0].Content {
		case "Add Black Sparrow is Sam. Add him.":
			result = toolReply("link_teammate", `{"discord_name":"Black Sparrow","sparrow_name":"Sam"}`)
		case "Link Night Owl to Rae":
			result = toolReply("link_teammate", `{"discord_name":"Night Owl","sparrow_name":"Rae"}`)
		case "Set Checkout as the default":
			result = toolReply("set_default_project", `{"project_key":"WP"}`)
		case "Set Operations as the default":
			result = toolReply("set_default_project", `{"project_key":"OPS"}`)
		case "Link me to Rae":
			result = toolReply("link_teammate", `{"discord_name":"me","sparrow_name":"Rae"}`)
		case "What's the status for today?":
			result = toolReply("today_status", `{}`)
		case "Today with empty project":
			result = toolReply("today_status", `{"project_key":""}`)
		case "List with null project":
			result = toolReply("list_issues", `{"project_key":null,"statuses":["todo"]}`)
		case "Create an oversized issue":
			result = toolReply("create_issue", fmt.Sprintf(`{"title":"Oversized issue","plan":%q}`, strings.Repeat("a", 1900)))
		case "Create a checkout issue":
			result = toolReply("create_issue", `{"title":"Investigate checkout"}`)
		case "Hold then create":
			select {
			case <-h.modelGate:
			case <-req.Context().Done():
				return
			}
			result = toolReply("create_issue", `{"title":"Checkout spinner","status":"todo","plan":"Check the spinner logs."}`)
		case "Find issues in Operations":
			if len(input.Messages) == 1 {
				result = toolReply("list_projects", `{}`)
			} else {
				transcript, _ := json.Marshal(input.Messages[2])
				if !bytes.Contains(transcript, []byte("Operations")) || !bytes.Contains(transcript, []byte("tool_result")) {
					t.Errorf("wrong discovery result: %s", transcript)
				}
				result = toolReply("list_issues", `{"project_key":"OPS"}`)
			}
		case "Unsupported":
			result = toolReply("delete_project", `{"project_key":"WP"}`)
		case "Bad issue lookup":
			result = toolReply("list_issues", `{"project_key":"MISSING"}`)
		default:
			result = toolReply("list_issues", `{"statuses":["todo"]}`)
		}
		_, _ = io.WriteString(w, result)
	}))
	t.Cleanup(anthropic.Close)
	endpoint.model = anthropicClient{key: "test-key", model: testModel, endpoint: anthropic.URL, client: anthropic.Client()}

	discordAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/guilds/"+guildID+"/members/search" {
			if req.Header.Get("Authorization") != "Bot test-token" {
				t.Error("member lookup did not use bot authentication")
			}
			switch req.URL.Query().Get("query") {
			case "Black Sparrow":
				_, _ = io.WriteString(w, `[{"nick":"Black Sparrow","user":{"id":"`+firstUser+`","username":"blacksparrow"}},{"nick":"Black Sparrow Two","user":{"id":"99"}}]`)
			case "Night Owl":
				_, _ = io.WriteString(w, `[{"nick":"Night Owl","user":{"id":"98"}},{"nick":"Night Owl","user":{"id":"99"}}]`)
			default:
				t.Error("unexpected Discord lookup")
			}
			return
		}
		if req.Method != http.MethodPost || req.URL.Path != "/channels/"+channelID+"/messages" || req.Header.Get("Authorization") != "Bot test-token" {
			t.Errorf("wrong channel reply request: %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var reply struct {
			message
			Reference    messageReference `json:"message_reference"`
			Nonce        string           `json:"nonce"`
			EnforceNonce bool             `json:"enforce_nonce"`
		}
		if err := json.NewDecoder(req.Body).Decode(&reply); err != nil {
			t.Errorf("decode channel reply: %v", err)
		}
		if reply.Reference.MessageID == "" || reply.Nonce != reply.Reference.MessageID || !reply.EnforceNonce || reply.Flags != 0 || reply.AllowedMentions.Parse == nil {
			t.Errorf("channel reply must reference the mention without pinging and use its ID as a nonce: %+v", reply)
		}
		h.replies <- reply.message
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(discordAPI.Close)
	endpoint.api, endpoint.client = discordAPI.URL, discordAPI.Client()
	h.bot = endpoint
	h.Server = httptest.NewServer(endpoint)
	t.Cleanup(h.Close)
	return h
}

func toolReply(name, args string) string {
	return fmt.Sprintf(`{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"tool_1","name":%q,"input":%s}]}`, name, args)
}

type testResult[T any] struct {
	value T
	err   error
}

func wrap[T any](value T, err error) testResult[T] { return testResult[T]{value, err} }

func must[T any](t *testing.T, value testResult[T]) T {
	t.Helper()
	if value.err != nil {
		t.Fatal(value.err)
	}
	return value.value
}

type discordReply struct {
	Type int     `json:"type"`
	Data message `json:"data"`
}

func (h *discordHarness) send(t *testing.T, body, timestamp string, valid bool, status int) discordReply {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2500*time.Millisecond)
	defer cancel()
	request := must(t, wrap(http.NewRequestWithContext(ctx, http.MethodPost, h.URL, strings.NewReader(body))))
	signature := ed25519.Sign(h.privateKey, []byte(timestamp+body))
	if !valid {
		signature[0] ^= 1
	}
	request.Header.Set("X-Signature-Timestamp", timestamp)
	request.Header.Set("X-Signature-Ed25519", hex.EncodeToString(signature))
	reply := must(t, wrap(h.Client().Do(request)))
	defer reply.Body.Close()
	if reply.StatusCode != status {
		raw, _ := io.ReadAll(reply.Body)
		t.Fatalf("status %d, want %d: %s", reply.StatusCode, status, raw)
	}
	var result discordReply
	if status == http.StatusOK {
		if err := json.NewDecoder(reply.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func (h *discordHarness) interact(t *testing.T, user, id string, kind int, data string) discordReply {
	t.Helper()
	return h.interactInGuild(t, guildID, user, id, kind, data)
}

func (h *discordHarness) interactInGuild(t *testing.T, sourceGuild, user, id string, kind int, data string) discordReply {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"type":%d,"application_id":%q,"guild_id":%q,"token":"test-token","member":{"user":{"id":%q}},"data":%s}`,
		id, kind, appID, sourceGuild, user, data)
	return h.send(t, body, strconv.FormatInt(h.now.Unix(), 10), true, http.StatusOK)
}

func (h *discordHarness) setup(t *testing.T, id, user, person, project string) {
	t.Helper()
	source := h.bot.guildSource(guildID, setupUser)
	input := store.DiscordSetup{InteractionID: id, UserID: user, PersonID: person, ProjectKey: project}
	if err := h.st.SaveDiscordSetup(t.Context(), source, input); err != nil {
		t.Fatal(err)
	}
}

func mentionEvent(user, id, content string) *discordgo.Message {
	return &discordgo.Message{
		ID: id, GuildID: guildID, ChannelID: channelID, Content: content,
		Author: &discordgo.User{ID: user}, Mentions: []*discordgo.User{{ID: appID}},
	}
}

func (h *discordHarness) ask(t *testing.T, user, id, prompt string) {
	t.Helper()
	h.askInGuild(t, guildID, user, id, prompt)
}

func (h *discordHarness) askInGuild(t *testing.T, sourceGuild, user, id, prompt string) {
	t.Helper()
	event := mentionEvent(user, id, "<@"+appID+"> "+prompt)
	event.GuildID = sourceGuild
	request, ok := h.bot.parseMention(event)
	if !ok {
		t.Fatal("valid Discord mention was rejected")
	}
	h.bot.respondToMention(t.Context(), request)
}

func (h *discordHarness) replied(t *testing.T) message {
	t.Helper()
	select {
	case reply := <-h.replies:
		return reply
	case <-time.After(5 * time.Second):
		t.Fatal("Sparrow did not reply to the channel message")
		return message{}
	}
}

func TestDiscordSignaturesAndScope(t *testing.T) {
	h := newDiscordHarness(t)
	timestamp := strconv.FormatInt(h.now.Unix(), 10)
	if reply := h.send(t, `{"type":1}`, timestamp, true, http.StatusOK); reply.Type != 1 {
		t.Fatalf("expected PONG, got %+v", reply)
	}
	h.send(t, `{"type":1}`, timestamp, false, http.StatusUnauthorized)
	h.send(t, `{"type":1}`, strconv.FormatInt(h.now.Add(-10*time.Minute).Unix(), 10), true, http.StatusUnauthorized)
	h.send(t, `{"id":"1","type":2,"application_id":"1234567890","guild_id":"999","member":{"user":{"id":"3456789012"}}}`, timestamp, true, http.StatusForbidden)
	h.send(t, `{"id":"1","type":2,"application_id":"1234567890","guild_id":"2345678901","member":{"user":{"id":"3456789012"}}}`, timestamp, true, http.StatusBadRequest)
	h.ask(t, firstUser, "1", "List issues")
	if reply := h.replied(t); !strings.Contains(reply.Content, "Link your Discord account") {
		t.Fatalf("unlinked account saw issue data: %+v", reply)
	}
}

func TestDiscordSetupCreateAndList(t *testing.T) {
	h := newDiscordHarness(t)
	h.setup(t, "12", firstUser, h.first.ID, "WP")
	h.setup(t, "13", secondUser, h.second.ID, "")
	h.setup(t, "12", firstUser, h.first.ID, "WP")

	h.modelGate <- struct{}{}
	h.ask(t, firstUser, "14", "Hold then create")
	draft := h.replied(t)
	if draft.Components == nil || !strings.Contains(draft.Content, "Checkout spinner") || !strings.Contains(draft.Content, "WP") {
		t.Fatalf("unexpected draft: %+v", draft)
	}
	project := must(t, wrap(h.st.GetProjectByKey(t.Context(), "WP")))
	issues := must(t, wrap(h.st.ListIssues(t.Context(), store.IssueFilter{ProjectID: project.ID})))
	if len(issues) != 0 {
		t.Fatal("draft already created an issue")
	}

	id := (*draft.Components)[0].Components[0].CustomID
	data := fmt.Sprintf(`{"custom_id":%q}`, id)
	unauthorized := h.interact(t, secondUser, "15", 3, data)
	if unauthorized.Type != 4 || !strings.Contains(unauthorized.Data.Content, "unavailable") {
		t.Fatalf("another user confirmed the draft: %+v", unauthorized)
	}
	for _, confirmID := range []string{"16", "17"} {
		reply := h.interact(t, firstUser, confirmID, 3, data)
		if reply.Type != 7 || !strings.Contains(reply.Data.Content, "Created **WP-1**") || reply.Data.Components == nil || len(*reply.Data.Components) != 0 {
			t.Fatalf("unexpected confirmation: %+v", reply)
		}
	}

	issue := must(t, wrap(h.st.GetIssueByKey(t.Context(), "WP-1")))
	actor := must(t, wrap(h.st.GetActor(t.Context(), issue.CreatedByID)))
	if actor.Kind != store.ActorAgent || actor.AgentName != "discord" || actor.OwnerActorID != h.first.ID {
		t.Fatalf("wrong creator: %+v", actor)
	}
	events := must(t, wrap(h.st.ListEvents(t.Context(), store.EventFilter{IssueID: issue.ID})))
	if len(events) != 1 || events[0].Kind != "issue.created" || events[0].ActorID != actor.ID || events[0].ReportedModel != testModel {
		t.Fatalf("wrong creation events: %+v", events)
	}
	if !bytes.Contains(events[0].Payload, []byte(`"discord_user_id":"`+firstUser+`"`)) {
		t.Fatalf("Discord user missing from audit event: %s", events[0].Payload)
	}
	allEvents := must(t, wrap(h.st.ListEvents(t.Context(), store.EventFilter{TargetType: store.TargetActor, TargetID: store.SystemActorID})))
	if len(allEvents) != 2 || !bytes.Contains(allEvents[0].Payload, []byte(setupUser)) {
		t.Fatalf("setup event was missing, duplicated, or unattributed: %+v", allEvents)
	}

	h.ask(t, secondUser, "18", "List issues")
	if reply := h.replied(t); !strings.Contains(reply.Content, "**WP-1** · Checkout spinner · todo") || reply.AllowedMentions.Parse == nil {
		t.Fatalf("wrong issue list: %+v", reply)
	}
	h.ask(t, secondUser, "19", "Find issues in Operations")
	if reply := h.replied(t); reply.Content != "No matching issues." || h.modelCalls.Load() != 4 {
		t.Fatalf("project discovery or explicit project override failed: %+v", reply)
	}
	h.ask(t, secondUser, "20", "Unsupported")
	if reply := h.replied(t); !strings.Contains(reply.Content, "report project status") {
		t.Fatalf("unexpected response to a disallowed tool: %+v", reply)
	}
	h.ask(t, secondUser, "21", "Bad issue lookup")
	if reply := h.replied(t); !strings.Contains(reply.Content, "not found") {
		t.Fatalf("expected a structured tool error: %+v", reply)
	}
}

func TestDiscordRemapRejectsPreviouslyPreparedDraft(t *testing.T) {
	h := newDiscordHarness(t)
	h.setup(t, "30", firstUser, h.first.ID, "WP")
	h.setup(t, "30b", secondUser, h.second.ID, "")
	third := must(t, wrap(h.st.CreateHuman(t.Context(), store.Caller{ActorID: store.SystemActorID}, store.NewHuman{DisplayName: "Kim", Email: "kim@example.com", Role: store.RoleMember})))
	h.modelGate <- struct{}{}
	h.ask(t, firstUser, "31", "Hold then create")
	draft := h.replied(t)

	if err := h.st.SaveDiscordSetup(t.Context(), h.bot.guildSource(guildID, setupUser), store.DiscordSetup{InteractionID: "32a", UserID: firstUser, PersonID: third.ID}); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("unlinked user remapped an existing link: %v", err)
	}
	if err := h.st.SaveDiscordSetup(t.Context(), h.bot.guildSource(guildID, secondUser), store.DiscordSetup{InteractionID: "32b", UserID: firstUser, PersonID: third.ID}); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("non-owner remapped another member's link: %v", err)
	}
	if err := h.st.SaveDiscordSetup(t.Context(), h.bot.guildSource(guildID, firstUser), store.DiscordSetup{InteractionID: "32c", UserID: secondUser, PersonID: third.ID}); err != nil {
		t.Fatalf("linked owner could not remap another member: %v", err)
	}
	if err := h.st.SaveDiscordSetup(t.Context(), h.bot.guildSource(guildID, firstUser), store.DiscordSetup{InteractionID: "32", UserID: firstUser, PersonID: h.second.ID}); err != nil {
		t.Fatalf("member could not remap themselves: %v", err)
	}

	id := (*draft.Components)[0].Components[0].CustomID
	reply := h.interact(t, firstUser, "33", 3, fmt.Sprintf(`{"custom_id":%q}`, id))
	if reply.Type != 4 || !strings.Contains(reply.Data.Content, "mapping changed") {
		t.Fatalf("stale draft used a previous mapping: %+v", reply)
	}
}

func TestDiscordNaturalSetupAndToday(t *testing.T) {
	h := newDiscordHarness(t)
	h.ask(t, setupUser, "50", "Add Black Sparrow is Sam. Add him.")
	if reply := h.replied(t); !strings.Contains(reply.Content, "Linked Black Sparrow to **Sam**") {
		t.Fatalf("did not link the unique exact member: %+v", reply)
	}
	h.ask(t, setupUser, "51", "Link Night Owl to Rae")
	if reply := h.replied(t); !strings.Contains(reply.Content, "More than one Discord member") {
		t.Fatalf("matched an ambiguous member: %+v", reply)
	}
	h.ask(t, setupUser, "52", "Set Checkout as the default")
	if reply := h.replied(t); !strings.Contains(reply.Content, "Checkout (WP)") {
		t.Fatalf("did not set a default project: %+v", reply)
	}
	links := must(t, wrap(h.st.ListDiscordLinks(t.Context())))
	if len(links) != 1 || links[0].UserID != firstUser || links[0].UserName != "Black Sparrow" || links[0].PersonID != h.first.ID {
		t.Fatalf("link did not record the member: %+v", links)
	}

	// A linked member's setup change is recorded as their Discord agent.
	h.ask(t, firstUser, "52b", "Set Operations as the default")
	h.replied(t)
	linked, project, err := h.st.DiscordCaller(t.Context(), h.bot.guildSource(guildID, firstUser), testModel)
	if err != nil || project != "OPS" {
		t.Fatalf("linked member could not change the default: %q %v", project, err)
	}
	events := must(t, wrap(h.st.ListEvents(t.Context(), store.EventFilter{ActorID: linked.ActorID})))
	if len(events) != 1 || events[0].Kind != "discord.setup" {
		t.Fatalf("setup change was not attributed to the Discord agent: %+v", events)
	}
	h.ask(t, setupUser, "52c", "Set Checkout as the default")
	h.replied(t)

	caller, project, err := h.st.DiscordCaller(t.Context(), h.bot.guildSource(guildID, firstUser), testModel)
	if err != nil || project != "WP" || caller.ActorID == "" {
		t.Fatalf("mapping and project were not persisted: %+v %q %v", caller, project, err)
	}
	h.ask(t, firstUser, "53", "Create a checkout issue")
	draft := h.replied(t)
	if draft.Components == nil || !strings.Contains(draft.Content, "Investigate checkout · backlog") {
		t.Fatalf("a natural description without a status did not produce a backlog draft: %+v", draft)
	}
	id := (*draft.Components)[0].Components[0].CustomID
	confirmed := h.interact(t, firstUser, "54", 3, fmt.Sprintf(`{"custom_id":%q}`, id))
	if confirmed.Type != 7 {
		t.Fatalf("did not confirm the draft: %+v", confirmed)
	}

	issue := must(t, wrap(h.st.GetIssueByKey(t.Context(), "WP-1")))
	if err := h.st.SetIssueStatus(t.Context(), store.Caller{ActorID: h.first.ID}, issue.ID, store.IssueInProgress); err != nil {
		t.Fatal(err)
	}
	h.ask(t, firstUser, "55", "What's the status for today?")
	status := h.replied(t).Content
	for _, want := range []string{"Checkout today", "1 in progress", "WP-1", "opened as backlog", "moved to in progress"} {
		if !strings.Contains(status, want) {
			t.Fatalf("missing %q in daily status: %s", want, status)
		}
	}
	_, outside, err := h.st.DiscordProjectStatus(t.Context(), issue.ProjectID, h.now.AddDate(0, 0, -2), h.now.AddDate(0, 0, -1))
	if err != nil || len(outside) != 0 {
		t.Fatalf("daily status included changes outside the requested day: %+v %v", outside, err)
	}
}

func TestDiscordMentionScope(t *testing.T) {
	h := newDiscordHarness(t, otherGuild)
	for _, tt := range []struct {
		name   string
		change func(*discordgo.Message)
	}{
		{"direct message", func(event *discordgo.Message) { event.GuildID = "" }},
		{"bot author", func(event *discordgo.Message) { event.Author.Bot = true }},
		{"webhook author", func(event *discordgo.Message) { event.WebhookID = "999" }},
		{"automatic reply ping", func(event *discordgo.Message) { event.Content = "List issues" }},
		{"mention missing from event", func(event *discordgo.Message) { event.Mentions = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			event := mentionEvent(firstUser, "1", "<@"+appID+"> List issues")
			tt.change(event)
			if _, ok := h.bot.parseMention(event); ok {
				t.Fatal("ignore anything but an explicit mention from a guild member")
			}
		})
	}
	request, ok := h.bot.parseMention(mentionEvent(firstUser, "2", "<@!"+appID+"> Link <@"+secondUser+"> to Rae"))
	if !ok || request.prompt != "Link <@"+secondUser+"> to Rae" || request.source.UserID != firstUser || request.channelID != channelID || request.messageID != "2" {
		t.Fatalf("wrong request (other user mentions must remain intact): %+v", request)
	}
}

func TestDiscordApprovedGuildsKeepSetupAndDraftsSeparate(t *testing.T) {
	h := newDiscordHarness(t, otherGuild)
	h.setup(t, "60", firstUser, h.first.ID, "WP")

	// A link in the first guild cannot authorize issue access in the second.
	h.askInGuild(t, otherGuild, firstUser, "61", "List issues")
	if reply := h.replied(t); !strings.Contains(reply.Content, "Link your Discord account") {
		t.Fatalf("the first guild's link leaked to the second: %+v", reply)
	}
	h.askInGuild(t, otherGuild, firstUser, "62", "Link me to Rae")
	if reply := h.replied(t); !strings.Contains(reply.Content, "**Rae**") {
		t.Fatalf("could not link a member in the second guild: %+v", reply)
	}
	h.askInGuild(t, otherGuild, firstUser, "63", "Set Operations as the default")
	if reply := h.replied(t); !strings.Contains(reply.Content, "Operations (OPS)") {
		t.Fatalf("could not set a default project in the second guild: %+v", reply)
	}
	caller, project, err := h.st.DiscordCaller(t.Context(), h.bot.guildSource(guildID, firstUser), testModel)
	if err != nil || project != "WP" {
		t.Fatalf("the second guild's setup changed the first guild's link or default: %+v %q %v", caller, project, err)
	}
	actor := must(t, wrap(h.st.GetActor(t.Context(), caller.ActorID)))
	if actor.OwnerActorID != h.first.ID {
		t.Fatalf("the second guild's link overwrote the first guild's actor: %+v", actor)
	}

	// Even the original author cannot confirm a draft from another approved guild.
	h.askInGuild(t, otherGuild, firstUser, "64", "Create a checkout issue")
	draft := h.replied(t)
	if draft.Components == nil || !strings.Contains(draft.Content, "OPS") {
		t.Fatalf("draft did not use the second guild's default: %+v", draft)
	}
	id := (*draft.Components)[0].Components[0].CustomID
	data := fmt.Sprintf(`{"custom_id":%q}`, id)
	if reply := h.interact(t, firstUser, "65", 3, data); reply.Type != 4 || !strings.Contains(reply.Data.Content, "unavailable") {
		t.Fatalf("draft confirmation crossed guilds: %+v", reply)
	}
	if reply := h.interactInGuild(t, otherGuild, firstUser, "66", 3, data); reply.Type != 7 {
		t.Fatalf("approval in the draft's guild failed: %+v", reply)
	}

	issue := must(t, wrap(h.st.GetIssueByKey(t.Context(), "OPS-1")))
	actor = must(t, wrap(h.st.GetActor(t.Context(), issue.CreatedByID)))
	if actor.OwnerActorID != h.second.ID {
		t.Fatalf("issue was attributed to the wrong guild's actor: %+v", actor)
	}
	events := must(t, wrap(h.st.ListEvents(t.Context(), store.EventFilter{IssueID: issue.ID})))
	if len(events) != 1 || events[0].ActorID != actor.ID || !bytes.Contains(events[0].Payload, []byte(`"discord_guild_id":"`+otherGuild+`"`)) {
		t.Fatalf("issue activity did not record the guild and actor: %+v", events)
	}
}

func TestDiscordUnknownGuildQueuesBeforeModelAndRequiresApproval(t *testing.T) {
	h := newDiscordHarness(t)
	ctx := t.Context()
	h.bot.observeGuild(ctx, thirdGuild, "Installed server")
	guilds := must(t, wrap(h.bot.GuildApprovals(ctx)))
	if len(guilds) != 2 || guilds[0].ID != thirdGuild || guilds[0].Name != "Installed server" || guilds[0].Status != store.DiscordGuildPending {
		t.Fatalf("observed server was not queued before any mention: %+v", guilds)
	}
	event := mentionEvent(firstUser, "70", "<@"+appID+"> List issues")
	event.GuildID = otherGuild
	request, ok := h.bot.parseMention(event)
	if !ok {
		t.Fatal("explicit mention could not request approval")
	}
	request.guildName = "Sandbox"
	h.bot.respondToMention(ctx, request)
	if reply := h.replied(t); !strings.Contains(reply.Content, "**Sandbox** needs approval") ||
		!strings.Contains(reply.Content, "Sparrow **Settings → Discord**") {
		t.Fatalf("unexpected pending reply: %+v", reply)
	}
	h.bot.respondToMention(ctx, request)
	select {
	case reply := <-h.replies:
		t.Fatalf("duplicate mention caused a pending reminder: %+v", reply)
	default:
	}
	if h.modelCalls.Load() != 0 {
		t.Fatal("pending server reached the model or its tools")
	}
	guilds = must(t, wrap(h.bot.GuildApprovals(ctx)))
	if len(guilds) != 3 || guilds[0].ID != otherGuild || guilds[0].Name != "Sandbox" || guilds[0].Status != store.DiscordGuildPending ||
		guilds[1].ID != thirdGuild || guilds[1].Name != "Installed server" || guilds[1].Status != store.DiscordGuildPending {
		t.Fatalf("unexpected queue: %+v", guilds)
	}
	must(t, wrap(h.bot.SetGuildApproval(ctx, store.Caller{ActorID: h.first.ID}, otherGuild, store.DiscordGuildApproved)))
	h.askInGuild(t, otherGuild, firstUser, "71", "Link me to Rae")
	if reply := h.replied(t); !strings.Contains(reply.Content, "**Rae**") || h.modelCalls.Load() != 1 {
		t.Fatalf("dynamically approved server could not use the bot: %+v", reply)
	}
	h.askInGuild(t, otherGuild, firstUser, "72", "Set Operations as the default")
	h.replied(t)
	h.askInGuild(t, otherGuild, firstUser, "73", "Create a checkout issue")
	draft := h.replied(t)
	if draft.Components == nil {
		t.Fatalf("dynamically approved server could not draft an issue: %+v", draft)
	}
	id := (*draft.Components)[0].Components[0].CustomID
	data := fmt.Sprintf(`{"custom_id":%q}`, id)
	if reply := h.interactInGuild(t, otherGuild, firstUser, "74", 3, data); reply.Type != 7 {
		t.Fatalf("dynamically approved server could not confirm a draft: %+v", reply)
	}
	must(t, wrap(h.bot.SetGuildApproval(ctx, store.Caller{ActorID: h.first.ID}, otherGuild, store.DiscordGuildRevoked)))
	signed := fmt.Sprintf(`{"id":"75","type":3,"application_id":%q,"guild_id":%q,"member":{"user":{"id":%q}},"data":%s}`, appID, otherGuild, firstUser, data)
	h.send(t, signed, strconv.FormatInt(h.now.Unix(), 10), true, http.StatusForbidden)
	h.now = h.now.Add(10 * time.Minute)
	h.bot.respondToMention(ctx, request)
	select {
	case reply := <-h.replies:
		t.Fatalf("revoked server replied to mention: %+v", reply)
	default:
	}
	if h.modelCalls.Load() != 3 {
		t.Fatal("revoked server reached the model or its tools")
	}
}

func TestDiscordEmptyProjectFallbackAndOversizedDraft(t *testing.T) {
	h := newDiscordHarness(t)
	ctx := t.Context()
	h.setup(t, "80", firstUser, h.first.ID, "WP")

	wp := must(t, wrap(h.st.GetProjectByKey(ctx, "WP")))
	ops := must(t, wrap(h.st.GetProjectByKey(ctx, "OPS")))
	owner := store.Caller{ActorID: h.first.ID}
	must(t, wrap(h.st.CreateIssue(ctx, owner, store.NewIssue{ProjectID: wp.ID, Title: "Checkout timeout", Status: store.IssueTodo})))
	must(t, wrap(h.st.CreateIssue(ctx, owner, store.NewIssue{ProjectID: ops.ID, Title: "Nightly backup", Status: store.IssueTodo})))

	h.ask(t, firstUser, "81", "Today with empty project")
	status := h.replied(t).Content
	if !strings.Contains(status, "Checkout today") || !strings.Contains(status, "WP-1") || strings.Contains(status, "OPS-1") {
		t.Fatalf("today_status with empty project_key did not fall back to default project: %s", status)
	}

	h.ask(t, firstUser, "82", "List with null project")
	list := h.replied(t).Content
	if !strings.Contains(list, "**WP-1** · Checkout timeout · todo") || strings.Contains(list, "OPS-1") {
		t.Fatalf("list_issues with null project_key did not fall back to default project: %s", list)
	}

	h.ask(t, firstUser, "83", "Create an oversized issue")
	reply := h.replied(t)
	if !strings.Contains(reply.Content, "too long to preview") || reply.Components != nil {
		t.Fatalf("oversized draft was not rejected cleanly: %+v", reply)
	}
	h.bot.draftMu.Lock()
	draftCount := len(h.bot.drafts)
	h.bot.draftMu.Unlock()
	if draftCount != 0 {
		t.Fatalf("oversized draft was stored in pending drafts: %d", draftCount)
	}
}
