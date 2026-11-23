import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { RefreshCw } from "lucide-react";
import { Input } from "../components/input";
import { Select } from "../components/select";
import { SettingsHeader } from "../components/settings-header";
import { apiWrite } from "../lib/api";
import { webUrl } from "../lib/format";
import type {
  Actor,
  GitHubAccount,
  GitHubChannel,
  GitHubInventory,
  GitHubRepository,
} from "../lib/types";
import { githubInventoryQuery } from "../queries/github";
import { actorsQuery, meQuery, projectsQuery } from "../queries/project";

const notLinked = "";

const panelClass =
  "rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]";
const buttonClass =
  "inline-flex items-center gap-2 rounded-full border border-[#303641] bg-[#14171c] px-[14px] py-[8px] text-[13px] font-[550] whitespace-nowrap text-[#d6dfed] transition-colors hover:border-[#484f5d] hover:bg-[#20242b] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
const primaryClass =
  "inline-flex items-center justify-center rounded-full border border-[#3264ff] bg-[#2455eb] px-[14px] py-[8px] text-[13px] font-[550] whitespace-nowrap text-white transition-colors hover:bg-[#3566f6] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
const dangerClass =
  "inline-flex items-center justify-center rounded-full border border-[#713440] bg-[#30191f] px-3 py-1.5 text-[12px] font-[550] whitespace-nowrap text-[#f2a9ae] transition-colors hover:border-[#a33a48] hover:bg-[#4a222b] focus-visible:border-[#f2a9ae] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
const errorClass =
  "mb-4 rounded-[8px] border border-[#5a2a33] bg-[#2a1519] px-4 py-[10px] text-[13px] text-[#f2a9ae]";
const headClass = "text-[13px] font-semibold tracking-normal text-[#e4eaf4] normal-case";
const badgeClass =
  "inline-flex items-center rounded-full border border-[#2b313a] bg-[#191c22] px-2.5 py-[3px] text-[12px] whitespace-nowrap text-[#c5cfdd]";

export const Route = createFileRoute("/settings_/github")({
  staticData: { section: "settings" },
  component: GithubSettingsPage,
});

/** Shows the GitHub app, repository, username, and Discord channel controls to the workspace owner. */
function GithubSettingsPage() {
  const identity = useQuery(meQuery);

  return (
    <>
      <SettingsHeader />
      <main className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        {identity.isPending && (
          <section className={panelClass}>
            <p role="status" className="m-0 text-[13px] text-[#a2aec2]">
              Loading GitHub settings…
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
            <GithubInventory />
          ) : (
            <section className={panelClass}>
              <h2>GitHub</h2>
              <p className="muted mt-[6px] text-[#a2aec2]">
                Only workspace owners can manage the GitHub integration.
              </p>
            </section>
          ))}
      </main>
    </>
  );
}

/** Loads the GitHub inventory once and shares it with the sections that edit it. */
function GithubInventory() {
  const client = useQueryClient();
  const inventory = useQuery(githubInventoryQuery);
  const refresh = useMutation({
    mutationFn: () => apiWrite<GitHubInventory>("/api/github/refresh", "POST"),
    onSuccess: (data) => {
      client.setQueryData(githubInventoryQuery.queryKey, data);
      void client.invalidateQueries({ queryKey: ["pull-requests"] });
    },
  });
  const data = inventory.data;

  return (
    <>
      {inventory.isPending && (
        <section className={panelClass}>
          <p role="status" className="m-0 text-[13px] text-[#a2aec2]">
            Loading GitHub settings…
          </p>
        </section>
      )}
      {inventory.error && (
        <p className={errorClass} role="alert">
          {inventory.error.message}
        </p>
      )}
      {data && (
        <>
          <AppStatus
            app={data.app}
            refreshing={refresh.isPending}
            error={refresh.error?.message}
            onRefresh={() => {
              refresh.reset();
              refresh.mutate();
            }}
          />
          {data.app.configured && (
            <>
              <Repositories repositories={data.repositories} />
              <Teammates accounts={data.accounts} />
              <Channels channels={data.channels} />
            </>
          )}
        </>
      )}
    </>
  );
}

function AppStatus({
  app,
  refreshing,
  error,
  onRefresh,
}: {
  app: GitHubInventory["app"];
  refreshing: boolean;
  error: string | undefined;
  onRefresh: () => void;
}) {
  const installUrl = app.install_url ? webUrl(app.install_url) : "";

  return (
    <section className={panelClass}>
      <div className="mb-4 flex items-start justify-between gap-[20px] [&_p]:mt-[3px]">
        <div>
          <h2>GitHub app</h2>
          <p className="muted text-[#a2aec2]">
            {app.configured
              ? "Connected. Pull requests from linked repositories appear on each project."
              : "Not configured."}
          </p>
        </div>
        {app.configured && (
          <div className="flex flex-wrap justify-end gap-2">
            {installUrl && (
              <a
                href={installUrl}
                target="_blank"
                rel="noreferrer noopener"
                className={`${primaryClass} border-[#3264ff] text-white`}
              >
                Install on GitHub
              </a>
            )}
            <button
              type="button"
              onClick={onRefresh}
              disabled={refreshing}
              aria-busy={refreshing}
              className={buttonClass}
            >
              <RefreshCw
                aria-hidden="true"
                className={`size-4 ${refreshing ? "animate-spin" : ""}`}
              />
              Refresh from GitHub
            </button>
          </div>
        )}
      </div>
      {error && (
        <p className={errorClass} role="alert">
          {error}
        </p>
      )}
      {!app.configured && (
        <p className="m-0 text-[13px] text-[#a2aec2]">
          The server needs the GITHUB_APP_ settings before it can connect to GitHub. Set them in the
          server configuration and restart it.
        </p>
      )}
    </section>
  );
}

/** Links each repository to a project and loads its recent pull requests. */
function Repositories({ repositories }: { repositories: GitHubRepository[] }) {
  const client = useQueryClient();
  const { data: projects = [] } = useQuery(projectsQuery);
  const [synced, setSynced] = useState<Record<number, number>>({});
  const link = useMutation({
    mutationFn: ({
      repository,
      projectKey,
    }: {
      repository: GitHubRepository;
      projectKey: string;
    }) =>
      apiWrite<{ repository: GitHubRepository }>(
        `/api/github/repositories/${repository.id}/project`,
        "PUT",
        { project_key: projectKey === notLinked ? null : projectKey },
      ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: githubInventoryQuery.queryKey });
      void client.invalidateQueries({ queryKey: ["pull-requests"] });
    },
  });
  const sync = useMutation({
    mutationFn: (repository: GitHubRepository) =>
      apiWrite<{ repository: GitHubRepository; synced: number }>(
        `/api/github/repositories/${repository.id}/sync`,
        "POST",
      ),
    onSuccess: (result, repository) => {
      setSynced((previous) => ({ ...previous, [repository.id]: result.synced }));
      void client.invalidateQueries({ queryKey: ["pull-requests"] });
    },
  });
  const options = [
    { value: notLinked, label: "Not linked" },
    ...projects.map((project) => ({ value: project.key, label: project.name })),
  ];

  return (
    <section className={panelClass}>
      <div className="mb-[20px] [&_p]:mt-[3px]">
        <h2>Repositories</h2>
        <p className="muted text-[#a2aec2]">
          Link a repository to a project to show its pull requests there.
        </p>
      </div>
      {link.error && (
        <p className={errorClass} role="alert">
          {link.error.message}
        </p>
      )}
      {sync.error && (
        <p className={errorClass} role="alert">
          {sync.error.message}
        </p>
      )}
      {repositories.length === 0 && (
        <p role="status" className="m-0 text-[13px] text-[#a2aec2]">
          No repositories yet. Install the app on GitHub, then refresh.
        </p>
      )}
      {repositories.length > 0 && (
        <div className="overflow-x-auto">
          <table>
            <thead>
              <tr>
                <th className={headClass}>Repository</th>
                <th className={`${headClass} w-[220px]`}>Project</th>
                <th className={`${headClass} w-[240px]`}>Pull requests</th>
              </tr>
            </thead>
            <tbody>
              {repositories.map((repository) => {
                const url = webUrl(repository.url);
                const loading = sync.isPending && sync.variables?.id === repository.id;
                const count = synced[repository.id];

                return (
                  <tr key={repository.id}>
                    <td>
                      <div className="flex flex-wrap items-center gap-2">
                        {url ? (
                          <a href={url} target="_blank" rel="noreferrer noopener">
                            {repository.full_name}
                          </a>
                        ) : (
                          <strong>{repository.full_name}</strong>
                        )}
                        {repository.private && <span className={badgeClass}>Private</span>}
                        <span className={badgeClass}>
                          {repository.installed ? "Installed" : "Not installed"}
                        </span>
                      </div>
                    </td>
                    <td>
                      <Select
                        label={`Project for ${repository.full_name}`}
                        value={repository.project_key ?? notLinked}
                        options={options}
                        onChange={(projectKey) => link.mutate({ repository, projectKey })}
                      />
                    </td>
                    <td>
                      <div className="flex items-center gap-[10px]">
                        <button
                          type="button"
                          onClick={() => sync.mutate(repository)}
                          disabled={sync.isPending || !repository.project_key}
                          aria-busy={loading}
                          aria-label={`Load pull requests for ${repository.full_name}`}
                          className={buttonClass}
                        >
                          Load pull requests
                        </button>
                        {count !== undefined && (
                          <span role="status" className="text-[13px] text-[#a2aec2]">
                            {count} loaded
                          </span>
                        )}
                      </div>
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

/** Lets an owner set the GitHub username that maps pull request authors to each teammate. */
function Teammates({ accounts }: { accounts: GitHubAccount[] }) {
  const { data: actors = [] } = useQuery(actorsQuery);
  const people = actors.filter((actor) => actor.kind === "human" && !actor.disabled_at);

  return (
    <section className={panelClass}>
      <div className="mb-[20px] [&_p]:mt-[3px]">
        <h2>Teammates’ GitHub usernames</h2>
        <p className="muted text-[#a2aec2]">
          Matches pull request authors and reviewers to people on the team.
        </p>
      </div>
      <div className="grid gap-[12px]">
        {people.map((person) => (
          <TeammateRow
            key={`${person.id}:${accounts.find((item) => item.person_id === person.id)?.login ?? ""}`}
            person={person}
            login={accounts.find((item) => item.person_id === person.id)?.login ?? ""}
          />
        ))}
      </div>
    </section>
  );
}

function TeammateRow({ person, login }: { person: Actor; login: string }) {
  const client = useQueryClient();
  const [draft, setDraft] = useState(login);
  const save = useMutation({
    mutationFn: (next: string) =>
      apiWrite<{ account: GitHubAccount | null }>(
        `/api/people/${encodeURIComponent(person.id)}/github`,
        "PUT",
        { login: next.trim() === "" ? null : next.trim() },
      ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: githubInventoryQuery.queryKey });
      void client.invalidateQueries({ queryKey: ["github-accounts"] });
      void client.invalidateQueries({ queryKey: ["pull-requests"] });
    },
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!save.isPending) save.mutate(draft);
  };

  return (
    <form onSubmit={submit} className="flex flex-wrap items-center gap-[12px]">
      <span className="w-[200px] max-[700px]:w-full">{person.display_name}</span>
      <Input
        aria-label={`GitHub username for ${person.display_name}`}
        placeholder="GitHub username"
        maxLength={39}
        value={draft}
        disabled={save.isPending}
        onChange={(event) => {
          setDraft(event.target.value);
          save.reset();
        }}
        className="w-[240px]"
      />
      <button
        type="submit"
        disabled={save.isPending || draft.trim() === login}
        className={buttonClass}
      >
        Save
      </button>
      {save.error && (
        <span role="alert" className="text-[13px] text-[#f2a9ae]">
          {save.error.message}
        </span>
      )}
    </form>
  );
}

/** Lists the Discord channels that receive pull request updates and lets an owner remove them. */
function Channels({ channels }: { channels: GitHubChannel[] }) {
  const client = useQueryClient();
  const { data: projects = [] } = useQuery(projectsQuery);
  const remove = useMutation({
    mutationFn: (channel: GitHubChannel) =>
      apiWrite<void>(`/api/github/channels/${encodeURIComponent(channel.project_key)}`, "DELETE"),
    onSuccess: () => client.invalidateQueries({ queryKey: githubInventoryQuery.queryKey }),
  });

  return (
    <section className={panelClass}>
      <div className="mb-[20px] [&_p]:mt-[3px]">
        <h2>Discord channels for pull request updates</h2>
        <p className="muted text-[#a2aec2]">
          Set a channel by mentioning the bot in it: “@Sparrow post GitHub updates for
          &lt;project&gt; here”.
        </p>
      </div>
      {remove.error && (
        <p className={errorClass} role="alert">
          {remove.error.message}
        </p>
      )}
      {channels.length === 0 && (
        <p role="status" className="m-0 text-[13px] text-[#a2aec2]">
          No channels are set.
        </p>
      )}
      {channels.length > 0 && (
        <div className="overflow-x-auto">
          <table>
            <thead>
              <tr>
                <th className={headClass}>Project</th>
                <th className={headClass}>Server</th>
                <th className={headClass}>Channel ID</th>
                <th className={`${headClass} w-[110px]`}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {channels.map((channel) => {
                const project =
                  projects.find((item) => item.key === channel.project_key)?.name ??
                  channel.project_key;

                return (
                  <tr key={channel.project_key}>
                    <td>{project}</td>
                    <td>{channel.guild_name || channel.guild_id}</td>
                    <td className="muted text-[#a2aec2]">{channel.channel_id}</td>
                    <td>
                      <button
                        type="button"
                        onClick={() => remove.mutate(channel)}
                        disabled={remove.isPending}
                        aria-label={`Remove the channel for ${project}`}
                        className={dangerClass}
                      >
                        Remove
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
