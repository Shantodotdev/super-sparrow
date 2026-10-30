import { createFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiWrite } from "../lib/api";
import { formatDate } from "../lib/format";
import { SettingsHeader } from "../components/settings-header";
import { useToast } from "../components/toast";

type Session = {
  id: string;
  device_label: string;
  current: boolean;
  last_seen_at: string;
  expires_at: string;
};

export const Route = createFileRoute("/settings_/sessions")({
  staticData: { section: "settings" },
  component: SessionsPage,
});

/** Lists real signed-in devices and revokes one or all sessions. */
function SessionsPage() {
  const client = useQueryClient();
  const { notify } = useToast();
  const sessions = useQuery({
    queryKey: ["sessions"],
    queryFn: async () => (await apiGet<{ sessions: Session[] }>("/api/sessions")).sessions,
  });
  const revoke = useMutation({
    mutationFn: (id: string) => apiWrite(id ? `/api/sessions/${id}` : "/api/sessions", "DELETE"),
    onSuccess: (_, id) => {
      if (!id || sessions.data?.find((s) => s.id === id)?.current) {
        client.clear();
        window.location.assign("/signin");
      } else void client.invalidateQueries({ queryKey: ["sessions"] });
    },
    onError: (error) => notify(error.message),
  });
  return (
    <>
      <SettingsHeader title="Signed-in devices" description="Manage your active sessions." />
      <section className="grid gap-4 p-8">
        {sessions.error && <p role="alert">{sessions.error.message}</p>}
        {sessions.isPending && <p>Loading devices…</p>}
        {sessions.data?.map((session) => (
          <div
            key={session.id}
            className="flex items-center justify-between border-b border-[#303641] p-4"
          >
            <div>
              <strong>{session.device_label}</strong>
              {session.current && " · Current"}
              <p>
                Last seen {formatDate(session.last_seen_at)} · Expires{" "}
                {formatDate(session.expires_at)}
              </p>
            </div>
            <button disabled={revoke.isPending} onClick={() => revoke.mutate(session.id)}>
              Sign out
            </button>
          </div>
        ))}
        <button
          disabled={revoke.isPending || !sessions.data?.length}
          onClick={() => revoke.mutate("")}
        >
          Sign out all devices
        </button>
      </section>
    </>
  );
}
