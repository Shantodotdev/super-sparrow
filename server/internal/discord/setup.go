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

// linkTeammate resolves names on their original services before writing an audited link.
func (h *Bot) linkTeammate(ctx context.Context, source store.DiscordIdentity, interactionID, userName, member, teammate string) (message, error) {
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
	memberName := shorten(strings.TrimPrefix(member, "@"), 100)
	switch {
	case userID == source.UserID:
		memberName = userName
	case userMention.MatchString(member):
		memberName = ""
	}
	if err := h.store.SaveDiscordSetup(ctx, source, store.DiscordSetup{
		InteractionID: interactionID, UserID: userID, UserName: memberName, PersonID: person.ID,
	}); err != nil {
		return message{}, err
	}
	return plain("Linked " + escape(member) + " to **" + escape(person.DisplayName) + "** in Sparrow."), nil
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
