import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { seedMembers, seedTokens } from "../data/team";
import { useToast } from "../components/toast";
import type { AgentName, AgentToken, TeamMember } from "../lib/types";
import { seededQuery } from "./seeded";

export const membersQuery = seededQuery("members", seedMembers);
export const tokensQuery = seededQuery("tokens", seedTokens);

/** Returns a function that adds a team member with no agents yet. */
export function useAddMember() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (values: { name: string; email: string }) => {
      const member: TeamMember = {
        ...values,
        role: "Member",
        agents: [],
        joined: "Just now",
      };

      queryClient.setQueryData(membersQuery.queryKey, (members) => [...(members ?? []), member]);
      notify("Added to this preview");
    },
    [queryClient, notify],
  );
}

/** Returns a function that creates an agent token owned by the signed-in member. */
export function useAddToken() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (values: { label: string; agent: AgentName }) => {
      const token: AgentToken = {
        id: String(Date.now()),
        owner: "Shanto",
        expires: "Dec 5, 2024",
        lastUsed: "Never",
        ...values,
      };

      queryClient.setQueryData(tokensQuery.queryKey, (tokens) => [...(tokens ?? []), token]);
      notify("Added to this preview");
    },
    [queryClient, notify],
  );
}

/** Returns a function that removes an agent token. */
export function useRevokeToken() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (id: string) => {
      queryClient.setQueryData(tokensQuery.queryKey, (tokens) =>
        tokens?.filter((token) => token.id !== id),
      );
      notify("Token removed from this preview");
    },
    [queryClient, notify],
  );
}
