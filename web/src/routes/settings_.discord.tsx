import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { RefreshCw } from "lucide-react";
import { SettingsHeader } from "../components/settings-header";
import { apiGet, apiWrite } from "../lib/api";
import { meQuery } from "../queries/project";

type DiscordGuildStatus = "pending" | "approved" | "revoked";

type DiscordGuild = {
  id: string;
  name: string;
  status: DiscordGuildStatus;
};

type DiscordInventory = {
  enabled: boolean;
  guilds: DiscordGuild[];
};

const inventoryQuery = queryOptions({
  queryKey: ["discord-guilds"],
  queryFn: () => apiGet<DiscordInventory>("/api/discord/guilds"),
  retry: false,
});

const statusMeta: Record<DiscordGuildStatus, { label: string; dotClass: string }> = {
  pending: { label: "Pending", dotClass: "bg-[#f2c464]" },
  approved: { label: "Approved", dotClass: "bg-[#16d99b]" },
  revoked: { label: "Revoked", dotClass: "bg-[#77889f]" },
};

const panelClass =
  "rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]";
const refreshClass =
  "inline-flex items-center gap-2 rounded-full border border-[#303641] bg-[#14171c] px-[14px] py-[8px] text-[13px] font-[550] whitespace-nowrap text-[#d6dfed] transition-colors hover:border-[#484f5d] hover:bg-[#20242b] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
const approveClass =
  "inline-flex min-w-[78px] items-center justify-center rounded-full border border-[#3264ff] bg-[#2455eb] px-3 py-1.5 text-[12px] font-[550] whitespace-nowrap text-white transition-[background-color,border-color,opacity] duration-150 hover:bg-[#3566f6] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
const revokeClass =
  "inline-flex min-w-[78px] items-center justify-center rounded-full border border-[#713440] bg-[#30191f] px-3 py-1.5 text-[12px] font-[550] whitespace-nowrap text-[#f2a9ae] transition-[background-color,border-color,opacity] duration-150 hover:border-[#a33a48] hover:bg-[#4a222b] focus-visible:border-[#f2a9ae] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
const errorClass =
  "mb-4 rounded-[8px] border border-[#5a2a33] bg-[#2a1519] px-4 py-[10px] text-[13px] text-[#f2a9ae]";

export const Route = createFileRoute("/settings_/discord")({
  staticData: { section: "settings" },
  component: DiscordSettingsPage,
});

function summarizeGuilds(guilds: DiscordGuild[]): string {
  if (guilds.length === 0) return "No servers connected";

  const counts = {
    pending: guilds.filter((guild) => guild.status === "pending").length,
    approved: guilds.filter((guild) => guild.status === "approved").length,
    revoked: guilds.filter((guild) => guild.status === "revoked").length,
  };
  const parts: string[] = [];
  if (counts.approved > 0) parts.push(`${counts.approved} approved`);
  if (counts.pending > 0) parts.push(`${counts.pending} pending`);
  if (counts.revoked > 0) parts.push(`${counts.revoked} revoked`);
  return parts.join(" · ");
}

/** Shows the Discord server approval controls to the workspace owner. */
function DiscordSettingsPage() {
  const identity = useQuery(meQuery);

  return (
    <>
      <SettingsHeader />
      <main className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        {identity.isPending && (
          <section className={panelClass}>
            <p role="status" className="m-0 text-[13px] text-[#a2aec2]">
              Loading Discord settings…
            </p>
          </section>
        )}
        {identity.isError && (
          <p className={errorClass} role="alert">
            {identity.error.message}
          </p>
        )}
        {identity.data &&
          (identity.data.role === "owner" ? (
            <DiscordServers />
          ) : (
            <section className={panelClass}>
              <h2>Discord servers</h2>
              <p className="muted mt-[6px] text-[#a2aec2]">
                Only workspace owners can manage Discord servers.
              </p>
            </section>
          ))}
      </main>
    </>
  );
}

/** Lists installed servers and updates access after the API saves an owner's decision. */
function DiscordServers() {
  const client = useQueryClient();
  const inventory = useQuery(inventoryQuery);
  const decide = useMutation({
    mutationFn: (guild: DiscordGuild) => {
      const action = guild.status === "approved" ? "revoke" : "approve";
      return apiWrite<{ guild: DiscordGuild }>(
        `/api/discord/guilds/${encodeURIComponent(guild.id)}/${action}`,
        "POST",
      );
    },
    onSuccess: ({ guild }) => {
      client.setQueryData(inventoryQuery.queryKey, (previous) => {
        if (!previous) return previous;
        return {
          ...previous,
          guilds: previous.guilds.map((item) => (item.id === guild.id ? guild : item)),
        };
      });
      return client.invalidateQueries({ queryKey: inventoryQuery.queryKey });
    },
  });

  const data = inventory.data;
  const guilds = data?.guilds ?? [];

  return (
    <section className={panelClass}>
      <div className="mb-[20px] flex items-start justify-between gap-[20px] [&_p]:mt-[3px]">
        <div>
          <h2>Discord servers</h2>
          {data?.enabled && <p className="muted text-[#a2aec2]">{summarizeGuilds(guilds)}</p>}
        </div>
        <button
          type="button"
          onClick={() => void inventory.refetch()}
          disabled={inventory.isFetching || decide.isPending}
          aria-busy={inventory.isFetching}
          className={refreshClass}
        >
          <RefreshCw
            aria-hidden="true"
            className={`size-4 ${inventory.isFetching ? "animate-spin" : ""}`}
          />
          Refresh
        </button>
      </div>

      {inventory.error && (
        <p className={errorClass} role="alert">
          {inventory.error.message}
        </p>
      )}
      {decide.error && (
        <p className={errorClass} role="alert">
          {decide.error.message}
        </p>
      )}

      {inventory.isPending && (
        <p role="status" className="m-0 text-[13px] text-[#a2aec2]">
          Loading Discord servers…
        </p>
      )}

      {data && !data.enabled && (
        <p className="m-0 text-[13px] text-[#a2aec2]">
          The Discord integration is disabled on this Sparrow server.
        </p>
      )}

      {data?.enabled && guilds.length === 0 && (
        <p role="status" className="m-0 text-[13px] text-[#a2aec2]">
          Add Super Sparrow to a Discord server using the bot’s install link, then refresh to
          approve it.
        </p>
      )}

      {data?.enabled && guilds.length > 0 && (
        <div className="overflow-x-auto">
          <table>
            <thead>
              <tr>
                <th className="text-[13px] font-semibold tracking-normal text-[#e4eaf4] normal-case">
                  Server
                </th>
                <th className="text-[13px] font-semibold tracking-normal text-[#e4eaf4] normal-case">
                  Server ID
                </th>
                <th className="w-40 text-[13px] font-semibold tracking-normal text-[#e4eaf4] normal-case">
                  Status
                </th>
                <th className="w-[110px] text-[13px] font-semibold tracking-normal text-[#e4eaf4] normal-case">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {guilds.map((guild) => {
                const { label, dotClass } = statusMeta[guild.status];
                const isApproved = guild.status === "approved";
                const action = isApproved ? "Revoke" : "Approve";
                const saving = decide.isPending && decide.variables?.id === guild.id;

                return (
                  <tr key={guild.id}>
                    <td>
                      <div className="flex items-center gap-[10px]">
                        <span className="flex size-8 flex-none items-center justify-center rounded-[8px] border border-[#222731] bg-[#171a1f]">
                          <img src="/brands/discord.svg" alt="" className="size-[17px]" />
                        </span>
                        <strong>{guild.name || "Discord server"}</strong>
                      </div>
                    </td>
                    <td className="muted text-[#a2aec2]">{guild.id}</td>
                    <td>
                      <span className="inline-flex items-center gap-2 rounded-full border border-[#2b313a] bg-[#191c22] px-2.5 py-[4px] text-[12px] whitespace-nowrap text-[#c5cfdd]">
                        <span className={`h-2 w-2 rounded-full ${dotClass}`} />
                        {label}
                      </span>
                    </td>
                    <td>
                      <button
                        type="button"
                        onClick={() => decide.mutate(guild)}
                        disabled={decide.isPending}
                        aria-busy={saving}
                        aria-label={`${action} ${guild.name || guild.id}`}
                        className={isApproved ? revokeClass : approveClass}
                      >
                        {action}
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
