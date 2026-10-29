package httpserver

import (
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// The views are the API's JSON shapes. They never hold a hash or a secret, except the new token
// secret that createToken returns once.

type actorView struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email,omitempty"`
	Role         string     `json:"role,omitempty"`
	AgentName    string     `json:"agent_name,omitempty"`
	OwnerActorID string     `json:"owner_actor_id,omitempty"`
	DisabledAt   *time.Time `json:"disabled_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func newActorView(actor store.Actor) actorView {
	return actorView{
		ID: actor.ID, Kind: string(actor.Kind), DisplayName: actor.DisplayName, Email: actor.Email,
		Role: string(actor.Role), AgentName: actor.AgentName, OwnerActorID: actor.OwnerActorID,
		DisabledAt: actor.DisabledAt, CreatedAt: actor.CreatedAt,
	}
}

type sessionView struct {
	ID          string    `json:"id"`
	DeviceLabel string    `json:"device_label"`
	Current     bool      `json:"current"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func newSessionView(session store.Session, currentID string) sessionView {
	return sessionView{
		ID: session.ID, DeviceLabel: session.DeviceLabel, Current: session.ID == currentID,
		LastSeenAt: session.LastSeenAt, ExpiresAt: session.ExpiresAt, CreatedAt: session.CreatedAt,
	}
}

type tokenView struct {
	ID           string     `json:"id"`
	AgentActorID string     `json:"agent_actor_id"`
	OwnerActorID string     `json:"owner_actor_id"`
	AgentName    string     `json:"agent_name"`
	Label        string     `json:"label"`
	DisplayHint  string     `json:"display_hint"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func newTokenView(token store.AgentToken) tokenView {
	return tokenView{
		ID: token.ID, AgentActorID: token.AgentActorID, OwnerActorID: token.OwnerActorID, AgentName: token.AgentName,
		Label: token.Label, DisplayHint: token.DisplayHint, LastUsedAt: token.LastUsedAt,
		ExpiresAt: token.ExpiresAt, CreatedAt: token.CreatedAt,
	}
}
