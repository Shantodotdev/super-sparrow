import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { MembersPanel } from "../components/members-panel";
import { SettingsDialog, type SettingsDialogKind } from "../components/settings-dialog";
import { SettingsHeader } from "../components/settings-header";
import { TokensPanel } from "../components/tokens-panel";
import { membersQuery, tokensQuery, useRevokeToken } from "../queries/team";

export const Route = createFileRoute("/settings")({
  staticData: { section: "settings" },
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...membersQuery, staleTime: "static" }),
      context.queryClient.query({ ...tokensQuery, staleTime: "static" }),
    ]);
  },
  component: SettingsPage,
});

/** Team members and agent tokens. */
function SettingsPage() {
  const { data: members } = useSuspenseQuery(membersQuery);
  const { data: tokens } = useSuspenseQuery(tokensQuery);
  const revokeToken = useRevokeToken();
  const [dialog, setDialog] = useState<SettingsDialogKind | null>(null);

  return (
    <>
      <SettingsHeader
        title="People and agents"
        description="Manage your team members, their agents, and agent tokens."
      />
      <div className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        <MembersPanel members={members} onAdd={() => setDialog("member")} />
        <TokensPanel tokens={tokens} onCreate={() => setDialog("token")} onRevoke={revokeToken} />
      </div>
      <SettingsDialog kind={dialog} onClose={() => setDialog(null)} />
    </>
  );
}
