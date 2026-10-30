import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../components/toast";
import { apiGet, apiWrite } from "../lib/api";
import type { AgentToken, NewAgentToken } from "../lib/types";
import { resourceQuery } from "./options";

export const tokensQuery = resourceQuery(
  "tokens",
  async () => (await apiGet<{ tokens: AgentToken[] }>("/api/tokens")).tokens,
);

export function useAddMember() {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: (v: { name: string; email: string }) =>
      apiWrite<{ invitation_sent: boolean }>("/api/people", "POST", {
        display_name: v.name,
        email: v.email,
        role: "member",
      }),
    onSuccess: () => client.invalidateQueries(),
  });
  return mutation.mutateAsync;
}

export function useCreateToken() {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: (v: { label: string; agentName: string; toolName: string }) =>
      apiWrite<NewAgentToken>("/api/tokens", "POST", { label: v.label, agent_name: v.agentName }),
    onSuccess: () => client.invalidateQueries(),
  });
  return mutation.mutateAsync;
}

export function useRevokeToken() {
  const client = useQueryClient();
  const { notify } = useToast();
  const mutation = useMutation({
    mutationFn: (id: string) => apiWrite(`/api/tokens/${id}`, "DELETE"),
    onSuccess: () => client.invalidateQueries(),
    onError: (error) => notify(error.message),
  });
  return mutation.mutate;
}
