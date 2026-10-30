import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { MembersPanel } from "../components/members-panel";
import { Modal } from "../components/modal";
import { SettingsDialog, type SettingsDialogKind } from "../components/settings-dialog";
import { SettingsHeader } from "../components/settings-header";
import { TokensPanel } from "../components/tokens-panel";
import type { AgentToken } from "../lib/types";
import { actorsQuery } from "../queries/project";
import { loadQuery } from "../queries/options";
import { tokensQuery, useRevokeToken } from "../queries/team";

export const Route = createFileRoute("/settings")({
  staticData: { section: "settings" },
  loader: async ({ context }) => {
    await Promise.all([
      loadQuery(context.queryClient, actorsQuery),
      loadQuery(context.queryClient, tokensQuery),
    ]);
  },
  component: SettingsPage,
});

/** Team members and agent tokens. Revoking a token asks first, because it cannot be undone. */
function SettingsPage() {
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const { data: tokens } = useSuspenseQuery(tokensQuery);
  const revokeToken = useRevokeToken();
  const [dialog, setDialog] = useState<SettingsDialogKind | null>(null);
  const [revoking, setRevoking] = useState<AgentToken | null>(null);

  const revokingAgent = actors.find((actor) => actor.id === revoking?.agent_actor_id);

  return (
    <>
      <SettingsHeader
        title="People and agents"
        description="Manage your team members, their agents, and agent tokens."
      />
      <div className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        <MembersPanel actors={actors} onAdd={() => setDialog("member")} />
        <TokensPanel tokens={tokens} onCreate={() => setDialog("token")} onRevoke={setRevoking} />
      </div>
      <SettingsDialog kind={dialog} onClose={() => setDialog(null)} />
      <Modal
        open={revoking !== null}
        onClose={() => setRevoking(null)}
        className="w-[min(100%,510px)] rounded-[10px] border border-[#303641] bg-[#12151a] p-[24px] shadow-[0_20px_70px_#000a] [&_h2]:mb-[12px] [&_p]:mb-[16px]"
        label="Revoke token"
      >
        <h2>Revoke “{revoking?.label}”?</h2>
        <p className="muted text-[#a2aec2]">
          {revokingAgent?.display_name ?? "The agent"} stops working with this token right away,
          including any open connection. This cannot be undone; create a new token to reconnect.
        </p>
        <div className="mt-[22px] flex justify-end gap-[8px]">
          <button
            autoFocus
            className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]"
            type="button"
            onClick={() => setRevoking(null)}
          >
            Cancel
          </button>
          <button
            className="rounded-full border border-[#a33a48] bg-[#7d1f2c] px-[15px] py-[10px] font-[550] text-white hover:bg-[#962636]"
            type="button"
            onClick={() => {
              if (revoking) revokeToken(revoking.id);
              setRevoking(null);
            }}
          >
            Revoke token
          </button>
        </div>
      </Modal>
    </>
  );
}
