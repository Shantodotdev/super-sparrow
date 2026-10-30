import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import { formatDate } from "../lib/format";

type Run = { result: string; error: string; started_at: string; finished_at: string | null };
type BackupStatus = {
  enabled: boolean;
  jobs: Record<"sync" | "daily", { last_run: Run | null; last_success: Run | null }>;
};

/** Shows backup health without exposing backup credentials. */
export function BackupStatusPanel() {
  const status = useQuery({
    queryKey: ["backups"],
    queryFn: () => apiGet<BackupStatus>("/api/backups"),
  });
  return (
    <section className="rounded-lg border border-[#222730] p-6">
      <h2>Backups</h2>
      {status.error && <p role="alert">{status.error.message}</p>}
      {status.isPending && <p>Loading backup status…</p>}
      {status.data &&
        (!status.data.enabled ? (
          <p>Backups are disabled on this server.</p>
        ) : (
          Object.entries(status.data.jobs).map(([job, state]) => (
            <div key={job}>
              <h3>{job === "sync" ? "Hourly sync" : "Daily backup"}</h3>
              <p>
                Last success:{" "}
                {state.last_success
                  ? formatDate(state.last_success.finished_at ?? state.last_success.started_at)
                  : "Never"}
              </p>
              {state.last_run?.result === "failed" && <p role="alert">{state.last_run.error}</p>}
            </div>
          ))
        ))}
    </section>
  );
}
