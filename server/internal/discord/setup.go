package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

type discordUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Bot        bool   `json:"bot"`
}

type discordMember struct {
	User discordUser `json:"user"`
	Nick string      `json:"nick"`
}

var userMention = regexp.MustCompile(`^<@!?(\d+)>$`)

// redeemLinkCode links the requester's own Discord account with a code they made in Sparrow.
func (h *Bot) redeemLinkCode(ctx context.Context, source store.DiscordIdentity, interactionID, userName, code string) (message, error) {
	if code == "" {
		return plain(linkCodeHelp), nil
	}
	hash, err := h.linkCodeHash(code)
	if err != nil {
		return plain(linkCodeHelp), nil
	}
	person, err := h.store.RedeemDiscordLinkCode(ctx, source, interactionID, userName, hash)
	if err != nil {
		return message{}, err
	}
	return plain("Linked your Discord account to **" + escape(person.DisplayName) + "** in Sparrow."), nil
}

const linkCodeHelp = "To link your Discord account, open Sparrow **Settings → Discord**, get a link code, and mention me with “link me with code BCDF-GHJK”."

// prepareLink resolves names on their original services and asks the owner to confirm the link.
// Only a requester linked to a workspace owner can link another member.
func (h *Bot) prepareLink(ctx context.Context, caller store.Caller, source store.DiscordIdentity, interactionID, member, teammate string) (message, error) {
	owner, err := h.store.DiscordCallerIsOwner(ctx, source)
	if err != nil {
		return message{}, err
	}
	if !owner {
		return plain("Only a workspace owner can link another Discord member. " + linkCodeHelp), nil
	}
	member, teammate = strings.TrimSpace(member), strings.TrimSpace(teammate)
	if member == "" || teammate == "" || len(member) > 150 || len(teammate) > 150 {
		return plain("Tell me the Discord username and the Sparrow teammate’s name you want to link."), nil
	}

	people, err := h.store.ListActors(ctx)
	if err != nil {
		return message{}, err
	}
	var exact, partial []store.Actor
	for _, person := range people {
		if person.Kind != store.ActorHuman || !person.Active() {
			continue
		}
		name := person.DisplayName
		if strings.EqualFold(name, teammate) {
			exact = append(exact, person)
		} else if strings.Contains(strings.ToLower(name), strings.ToLower(teammate)) {
			partial = append(partial, person)
		}
	}
	if len(exact) == 0 {
		exact = partial
	}
	if len(exact) == 0 {
		return plain("I couldn’t find an active Sparrow teammate named " + escape(teammate) + "."), nil
	}
	if len(exact) != 1 {
		return plain("More than one Sparrow teammate matches " + escape(teammate) + ". Try their full display name."), nil
	}

	person := exact[0]
	userID, reply, err := h.resolveMember(ctx, source, member)
	if err != nil || userID == "" {
		return plain(reply), err
	}
	return h.offerDraft(interactionID, draft{caller: caller, setup: &setupDraft{
		Member: store.DiscordMember{UserID: userID, Name: shorten(member, 100)}, PersonID: person.ID, PersonName: person.DisplayName,
	}}), nil
}

// resolveMember accepts the signed caller for “me”, or checks a mention or exact name with Discord.
func (h *Bot) resolveMember(ctx context.Context, source store.DiscordIdentity, name string) (string, string, error) {
	if strings.EqualFold(name, "me") {
		return source.UserID, "", nil
	}

	route := "/guilds/" + source.GuildID + "/members"
	var members []discordMember
	var status int
	var err error
	if mention := userMention.FindStringSubmatch(name); mention != nil {
		if mention[1] == source.UserID {
			return source.UserID, "", nil
		}
		var member discordMember
		status, err = h.discordGET(ctx, route+"/"+mention[1], &member)
		if status == http.StatusOK && err == nil {
			members = []discordMember{member}
		}
	} else {
		name = strings.TrimPrefix(name, "@")
		if name == "" {
			return "", "Give me their Discord username or server nickname.", nil
		}
		params := url.Values{"query": {name}, "limit": {"100"}}
		var matches []discordMember
		status, err = h.discordGET(ctx, route+"/search?"+params.Encode(), &matches)
		for _, match := range matches {
			if strings.EqualFold(match.Nick, name) || strings.EqualFold(match.User.Username, name) || strings.EqualFold(match.User.GlobalName, name) {
				members = append(members, match)
			}
		}
	}
	if err != nil {
		return "", "", err
	}
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return "", "I couldn’t look up that member. Make sure the Sparrow bot is installed in this server. To link your own account, say “link me” and give your Sparrow name.", nil
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("look up Discord member: HTTP %d", status)
	}

	var ids []string
	for _, member := range members {
		if member.User.ID != "" && !member.User.Bot {
			ids = append(ids, member.User.ID)
		}
	}
	switch len(ids) {
	case 1:
		return ids[0], "", nil
	case 0:
		return "", "I couldn’t find a Discord member named " + escape(name) + ". Try their exact username, server nickname, or a mention.", nil
	default:
		return "", "More than one Discord member matches " + escape(name) + ". Try their exact username or mention them.", nil
	}
}

// discordGET reads one Discord resource with bot authentication.
func (h *Bot) discordGET(ctx context.Context, path string, result any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.api+path, nil)
	if err != nil {
		return 0, fmt.Errorf("build Discord lookup: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+h.cfg.BotToken)
	reply, err := h.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("call Discord member lookup: %w", err)
	}
	defer reply.Body.Close()
	if reply.StatusCode != http.StatusOK {
		return reply.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(reply.Body, 256<<10)).Decode(result); err != nil {
		return reply.StatusCode, fmt.Errorf("decode Discord member lookup: %w", err)
	}
	return reply.StatusCode, nil
}
