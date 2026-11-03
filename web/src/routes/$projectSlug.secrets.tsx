import { useEffect, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { PageHeader } from "../components/page-header";
import { PasteEnvDialog } from "../components/paste-env-dialog";
import { RequiredKeysDialog } from "../components/required-keys-dialog";
import { SecretEditDialog } from "../components/secret-edit-dialog";
import { SecretHistoryModal } from "../components/secret-history-modal";
import { SecretsAccessLog } from "../components/secrets-access-log";
import { SecretsReviewDialog } from "../components/secrets-review-dialog";
import { SecretsTable } from "../components/secrets-table";
import { Badge, buttonClass, ErrorText, Panel, primaryClass } from "../components/secrets-ui";
import { ServiceTokensPanel } from "../components/service-tokens-panel";
import { Select } from "../components/select";
import { Tabs } from "../components/tabs";
import { useToast } from "../components/toast";
import { useProject } from "../hooks/use-project";
import { ApiError, apiGetText } from "../lib/api";
import {
  canDownloadConfig,
  canEditConfig,
  canViewConfig,
  downloadFileName,
  downloadFormatLabel,
  downloadFormats,
  noPendingChanges,
  patchBody,
  pendingCount,
  removePending,
  secretConfigLabel,
  secretConfigs,
  setPending,
  summarizePending,
  undoPending,
  type DownloadFormat,
  type PendingChanges,
} from "../lib/secrets";
import type { SecretConfigId, SecretsOverview } from "../lib/types";
import { loadQuery } from "../queries/options";
import { actorsQuery, projectsQuery } from "../queries/project";
import {
  downloadPath,
  refreshAccessLog,
  secretsQuery,
  useEnableSecrets,
  usePatchSecrets,
} from "../queries/secrets";

type SecretsSearch = { config?: SecretConfigId };

export const Route = createFileRoute("/$projectSlug/secrets")({
  staticData: { section: "secrets" },
  validateSearch: (search: Record<string, unknown>): SecretsSearch => ({
    config: secretConfigs.find((config) => config === search.config),
  }),
  loader: async ({ context, params }) => {
    const projects = await loadQuery(context.queryClient, projectsQuery);
    await loadQuery(context.queryClient, actorsQuery);
    const project = projects.find((item) => item.slug === params.projectSlug);
    if (project) void context.queryClient.prefetchQuery(secretsQuery(project.key));
  },
  component: SecretsPage,
});

/** The explanation shown while secrets are off: owners can turn them on, everyone else is told who can. */
function SecretsOff({ projectKey, isOwner }: { projectKey: string; isOwner: boolean }) {
  const enable = useEnableSecrets(projectKey);

  return (
    <div className="mx-auto my-24 grid max-w-120 justify-items-center gap-3 rounded-xl border border-dashed border-[#2b3039] px-6 py-12 text-center">
      <h2>Secrets are off for this project</h2>
      <p className="text-sm text-[#9ba9be]">
        Keep dev, staging, and production settings here, encrypted, with a history of every change.
        Each person gets their own dev on top of shared team defaults.
      </p>
      {isOwner ? (
        <button
          type="button"
          className={primaryClass}
          disabled={enable.isPending}
          onClick={() => enable.mutate()}
        >
          Turn on secrets
        </button>
      ) : (
        <p className="text-sm text-[#9ba9be]">An owner of the project can turn secrets on.</p>
      )}
      {enable.error && <ErrorText>{enable.error.message}</ErrorText>}
    </div>
  );
}

const objectUrlLifetime = 10_000;

/** Saves a text download as a file through a temporary link. */
function saveFile(text: string, fileName: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), objectUrlLifetime);
}

/** The Secrets page: tabs per config, the key table with pending edits, service tokens, and the access log. */
function SecretsPage() {
  const project = useProject();
  const overview = useQuery(secretsQuery(project.key));

  return (
    <>
      <PageHeader
        crumb={project.name}
        title="Secrets"
        description="Settings and keys for dev, staging, and production."
      />
      {overview.error && (
        <div className="px-[32px] py-[20px]">
          <ErrorText>{overview.error.message}</ErrorText>
        </div>
      )}
      {overview.isPending && (
        <p role="status" className="px-[32px] py-12 text-sm text-[#9ba9be]">
          Loading secrets…
        </p>
      )}
      {overview.data &&
        (overview.data.enabled && overview.data.configs ? (
          <SecretsContent
            key={project.key}
            projectKey={project.key}
            projectSlug={project.slug}
            overview={overview.data}
          />
        ) : (
          <SecretsOff projectKey={project.key} isOwner={overview.data.is_owner} />
        ))}
    </>
  );
}

/** The Secrets page body for one project; the page keys it by project so edits never cross projects. */
function SecretsContent({
  projectKey,
  projectSlug,
  overview,
}: {
  projectKey: string;
  projectSlug: string;
  overview: SecretsOverview;
}) {
  const { notify } = useToast();
  const client = useQueryClient();
  const search = Route.useSearch();
  const config = search.config ?? "dev";
  const listing = overview.configs![config];
  const canEdit = canEditConfig(config, overview.can_manage);
  const canView = canViewConfig(config, overview.can_manage);
  const canDownload = canDownloadConfig(config, overview.can_manage);

  const patch = usePatchSecrets(projectKey, config);
  const [pendingByConfig, setPendingByConfig] = useState<
    Partial<Record<SecretConfigId, PendingChanges>>
  >({});
  const [editing, setEditing] = useState<{ open: boolean; name: string }>({
    open: false,
    name: "",
  });
  const [pasting, setPasting] = useState(false);
  const [reviewing, setReviewing] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [requiredOpen, setRequiredOpen] = useState(false);
  const [historyName, setHistoryName] = useState("");
  const [downloading, setDownloading] = useState(false);
  const [format, setFormat] = useState<DownloadFormat>("env");

  const pending = pendingByConfig[config] ?? noPendingChanges;
  const saved = listing.secrets.map((entry) => entry.name);
  const summary = summarizePending(pending, saved);
  const count = pendingCount(pending);
  const hasUnsaved = secretConfigs.some(
    (id) => pendingCount(pendingByConfig[id] ?? noPendingChanges) > 0,
  );
  const defaultNames = listing.secrets
    .filter((entry) => entry.source === "default")
    .map((entry) => entry.name);
  const overrides = summary.changed.filter((name) => defaultNames.includes(name));

  useEffect(() => {
    if (!hasUnsaved) return;

    const beforeUnload = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", beforeUnload);

    return () => window.removeEventListener("beforeunload", beforeUnload);
  }, [hasUnsaved]);

  const update = (change: (current: PendingChanges) => PendingChanges) =>
    setPendingByConfig((all) => ({ ...all, [config]: change(all[config] ?? noPendingChanges) }));

  const closeReview = () => {
    setReviewing(false);
    setConfirmed(false);
    patch.reset();
  };

  const save = () =>
    patch.mutate(patchBody(pending), {
      onSuccess: () => {
        setPendingByConfig((all) => ({ ...all, [config]: noPendingChanges }));
        closeReview();
        notify(`Saved changes to ${secretConfigLabel[config]}`);
      },
    });

  const download = async () => {
    setDownloading(true);
    try {
      saveFile(
        await apiGetText(downloadPath(projectKey, config, format)),
        downloadFileName(projectSlug, config, format),
      );
    } catch (error) {
      if (format === "env" && error instanceof ApiError && error.status === 400) {
        setFormat("shell");
        notify(
          `${error.message} Use the Shell script or JSON format instead; Shell script is selected now.`,
        );
      } else {
        notify(error instanceof Error ? error.message : "Could not download the config");
      }
    } finally {
      setDownloading(false);
      refreshAccessLog(client, projectKey);
    }
  };

  return (
    <div className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:px-[16px]">
      <Tabs className="max-[700px]:flex-wrap max-[700px]:gap-x-[18px]">
        {secretConfigs.map((id) => (
          <Link
            key={id}
            to="/$projectSlug/secrets"
            params={{ projectSlug }}
            search={{ config: id }}
            activeProps={{}}
            className={id === config ? "selected" : ""}
          >
            {secretConfigLabel[id]}
            {pendingCount(pendingByConfig[id] ?? noPendingChanges) > 0 && (
              <>
                <span aria-hidden="true"> •</span>
                <span className="sr-only"> (unsaved changes)</span>
              </>
            )}
          </Link>
        ))}
      </Tabs>
      {listing.missing.length > 0 && (
        <div
          role="alert"
          className="rounded-lg border border-[#6b5420] bg-[#2a2210] p-4 text-sm text-[#f0cf75]"
        >
          Missing required {listing.missing.length === 1 ? "key" : "keys"} in{" "}
          {secretConfigLabel[config]}:{" "}
          <span className="font-mono">{listing.missing.join(", ")}</span>
        </div>
      )}
      <Panel
        title={secretConfigLabel[config]}
        description={
          config === "dev"
            ? "Team defaults plus your own overrides. Only you and your agents see your overrides."
            : config === "dev-defaults"
              ? "Defaults every teammate’s dev starts from."
              : `Shared ${secretConfigLabel[config].toLowerCase()} settings for the team.`
        }
        action={
          <div className="flex flex-wrap items-center justify-end gap-2">
            {canEdit && (
              <>
                <button type="button" className={buttonClass} onClick={() => setPasting(true)}>
                  Paste .env
                </button>
                <button
                  type="button"
                  className={primaryClass}
                  onClick={() => setEditing({ open: true, name: "" })}
                >
                  Add key
                </button>
              </>
            )}
            {canDownload && (
              <>
                <Select
                  label="Download format"
                  value={format}
                  onChange={(value) => setFormat(value as DownloadFormat)}
                  options={downloadFormats.map((value) => ({
                    value,
                    label: downloadFormatLabel[value],
                  }))}
                />
                <button
                  type="button"
                  className={buttonClass}
                  disabled={downloading}
                  onClick={() => void download()}
                >
                  Download
                </button>
              </>
            )}
            {overview.can_manage && (
              <button type="button" className={buttonClass} onClick={() => setRequiredOpen(true)}>
                Required keys
              </button>
            )}
          </div>
        }
      >
        {!canView && (
          <p className="mb-4 text-sm text-[#a2aec2]">
            You can see key names only. Ask an owner for permission to manage staging and production
            to read or change values.
          </p>
        )}
        {count > 0 && (
          <div className="mb-4 flex flex-wrap items-center gap-3 rounded-lg border border-[#2f4a99] bg-[#16213f] p-3 text-sm">
            <Badge tone="blue">Unsaved</Badge>
            <span>
              {count} {count === 1 ? "change" : "changes"} not saved yet
            </span>
            <div className="ml-auto flex gap-2">
              <button
                type="button"
                className={buttonClass}
                onClick={() => update(() => noPendingChanges)}
              >
                Discard
              </button>
              <button type="button" className={primaryClass} onClick={() => setReviewing(true)}>
                Review and save
              </button>
            </div>
          </div>
        )}
        <SecretsTable
          key={`${projectKey}:${config}`}
          projectKey={projectKey}
          config={config}
          listing={listing}
          canView={canView}
          canEdit={canEdit}
          canManage={overview.can_manage}
          pending={pending}
          onEdit={(name) => setEditing({ open: true, name })}
          onRemove={(name) => update((current) => removePending(current, name, saved))}
          onUndo={(name) => update((current) => undoPending(current, name))}
          onHistory={setHistoryName}
        />
      </Panel>
      {overview.can_manage && <ServiceTokensPanel projectKey={projectKey} />}
      <SecretsAccessLog projectKey={projectKey} isOwner={overview.is_owner} />
      <SecretEditDialog
        open={editing.open}
        name={editing.name}
        pendingValue={pending.set[editing.name]}
        defaultNames={defaultNames}
        onClose={() => setEditing((current) => ({ ...current, open: false }))}
        onApply={(name, value) => update((current) => setPending(current, name, value))}
      />
      <PasteEnvDialog
        open={pasting}
        onClose={() => setPasting(false)}
        onApply={(entries) =>
          update((current) =>
            entries.reduce((next, entry) => setPending(next, entry.name, entry.value), current),
          )
        }
      />
      <SecretsReviewDialog
        open={reviewing}
        config={config}
        summary={summary}
        overrides={overrides}
        confirmed={confirmed}
        saving={patch.isPending}
        error={patch.error?.message ?? ""}
        onConfirmedChange={setConfirmed}
        onClose={closeReview}
        onConfirm={save}
      />
      <RequiredKeysDialog
        open={requiredOpen}
        projectKey={projectKey}
        config={config}
        names={listing.required}
        onClose={() => setRequiredOpen(false)}
      />
      <SecretHistoryModal
        projectKey={projectKey}
        config={config}
        name={historyName}
        canRestore={canEdit}
        onClose={() => setHistoryName("")}
      />
    </div>
  );
}
