import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { currentPerson, seedTokens } from "../data/team";
import { useToast } from "../components/toast";
import type { Actor, AgentToken, NewAgentToken } from "../lib/types";
import { actorsQuery } from "./project";
import { seededQuery } from "./seeded";

export const tokensQuery = seededQuery("tokens", seedTokens);

/** Returns a function that adds a team member, as an owner's invitation does. */
export function useAddMember() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (values: { name: string; email: string }) => {
      const member: Actor = {
        id: `person-${Date.now()}`,
        kind: "human",
        display_name: values.name,
        email: values.email,
        role: "member",
        disabled_at: null,
        created_at: new Date().toISOString(),
      };

      queryClient.setQueryData(actorsQuery.queryKey, (actors) => [...(actors ?? []), member]);
      notify("Added to this preview");
    },
    [queryClient, notify],
  );
}

/** Makes a token secret the way the server does: ssp_ and 43 random URL-safe characters. */
function previewSecret(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  return `ssp_${btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "")}`;
}

/**
 * Returns a function that creates a token for the signed-in person's agent for a tool, adding the
 * agent the first time, as POST /api/tokens does. It returns the secret, which is shown once.
 */
export function useCreateToken() {
  const queryClient = useQueryClient();

  return useCallback(
    (values: { label: string; agentName: string; toolName: string }): NewAgentToken => {
      const actors = queryClient.getQueryData(actorsQuery.queryKey) ?? [];
      let agent = actors.find(
        (actor) =>
          actor.owner_actor_id === currentPerson.id && actor.agent_name === values.agentName,
      );
      if (!agent) {
        agent = {
          id: `agent-${currentPerson.id}-${values.agentName}`,
          kind: "agent",
          display_name: values.toolName,
          agent_name: values.agentName,
          owner_actor_id: currentPerson.id,
          disabled_at: null,
          created_at: new Date().toISOString(),
        };
        queryClient.setQueryData(actorsQuery.queryKey, [...actors, agent]);
      }

      const secret = previewSecret();
      const now = new Date();
      const token: AgentToken = {
        id: `token-${now.getTime()}`,
        agent_actor_id: agent.id,
        owner_actor_id: currentPerson.id,
        agent_name: values.agentName,
        label: values.label,
        display_hint: `ssp_…${secret.slice(-4)}`,
        last_used_at: null,
        expires_at: new Date(now.getTime() + 90 * 24 * 60 * 60 * 1000).toISOString(),
        created_at: now.toISOString(),
      };

      queryClient.setQueryData(tokensQuery.queryKey, (tokens) => [token, ...(tokens ?? [])]);
      return { token, secret };
    },
    [queryClient],
  );
}

/** Returns a function that revokes an agent token. */
export function useRevokeToken() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (id: string) => {
      queryClient.setQueryData(tokensQuery.queryKey, (tokens) =>
        tokens?.filter((token) => token.id !== id),
      );
      notify("Token revoked in this preview");
    },
    [queryClient, notify],
  );
}
