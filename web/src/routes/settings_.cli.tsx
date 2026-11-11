import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { Input } from "../components/input";
import { SettingsHeader } from "../components/settings-header";
import { ApiError, apiWrite } from "../lib/api";
import { formatCliCode, isCompleteCliCode, validateCliSearch } from "../lib/cli-code";
import { formatRelative } from "../lib/format";
import { cliLoginRequestQuery } from "../queries/cli-login";

export const Route = createFileRoute("/settings_/cli")({
  staticData: { section: "settings" },
  validateSearch: validateCliSearch,
  component: ConnectCliPage,
});

type Decision = "approve" | "deny";

const cardClass =
  "rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]";
const buttonClass =
  "rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b] disabled:cursor-not-allowed disabled:opacity-50";
const primaryClass =
  "rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6] disabled:cursor-not-allowed disabled:opacity-50";
const dangerClass =
  "rounded-full border border-[#713440] bg-[#30191f] px-[15px] py-[10px] font-[550] text-[#f2a9ae] hover:border-[#a33a48] hover:bg-[#4a222b] disabled:cursor-not-allowed disabled:opacity-50";
const errorClass =
  "rounded-[8px] border border-[#5a2a33] bg-[#2a1519] px-4 py-[10px] text-[13px] text-[#f2a9ae]";

const expiredMessage = "This code has expired or was already used. Run sparrow login again.";

/** Whether the error says the server no longer knows the request. */
function isExpired(error: Error | null): boolean {
  return error instanceof ApiError && error.status === 404;
}

/** Approves a `sparrow login` from the CLI, or takes the code to look one up, and explains how to set up another computer. */
function ConnectCliPage() {
  const { code } = Route.useSearch();

  return (
    <>
      <SettingsHeader />
      <main className="mx-auto grid w-full max-w-[680px] gap-[22px] px-[32px] pt-[8vh] pb-[20px] max-[700px]:p-[16px]">
        {code ? <CliRequestReview key={code} code={code} /> : <CliCodeForm />}
        <NewComputerHelp />
      </main>
    </>
  );
}

/** Asks for the code the terminal printed, then moves to the review by putting it in the address. */
function CliCodeForm() {
  const navigate = useNavigate();
  const [typed, setTyped] = useState("");

  const submit = (event: FormEvent) => {
    event.preventDefault();

    if (!isCompleteCliCode(typed)) return;
    void navigate({ to: "/settings/cli", search: { code: typed } });
  };

  return (
    <form onSubmit={submit} className={`${cardClass} grid gap-4`}>
      <div>
        <h2>Connect the CLI</h2>
        <p className="muted mt-[3px] text-[#a2aec2]">
          Run sparrow login in a terminal, then type the code it shows.
        </p>
      </div>
      <label>
        Code from the terminal
        <Input
          autoFocus
          required
          autoComplete="off"
          autoCapitalize="characters"
          spellCheck={false}
          placeholder="XXXX-XXXX"
          maxLength={9}
          value={typed}
          onChange={(event) => setTyped(formatCliCode(event.target.value))}
          className="w-full rounded-[10px] px-4 py-3 font-mono text-[22px] tracking-[0.2em]"
        />
      </label>
      <div>
        <button disabled={!isCompleteCliCode(typed)} className={primaryClass}>
          Continue
        </button>
      </div>
    </form>
  );
}

/** Shows who is asking to sign in with this code and lets the person approve or deny it. */
function CliRequestReview({ code }: { code: string }) {
  const client = useQueryClient();
  const request = useQuery(cliLoginRequestQuery(code));
  const decide = useMutation({
    mutationFn: (decision: Decision) =>
      apiWrite<{ status: "approved" | "denied" }>(
        `/api/auth/cli/requests/${encodeURIComponent(code)}/${decision}`,
        "POST",
      ),
    onSuccess: () => client.invalidateQueries({ queryKey: ["sessions"] }),
  });

  const device = request.data?.device_label;
  const status = decide.data?.status;
  const error = request.error ?? decide.error;

  if (request.isPending) return <p className={cardClass}>Looking up the code…</p>;

  if (isExpired(error)) {
    return (
      <section className={`${cardClass} grid gap-4`} role="alert">
        <p>{expiredMessage}</p>
        <div>
          <Link to="/settings/cli" search={{ code: undefined }} className={buttonClass}>
            Enter another code
          </Link>
        </div>
      </section>
    );
  }

  if (status === "approved") {
    return (
      <section className={cardClass} role="status">
        <h2>Approved</h2>
        <p className="muted mt-[3px] text-[#a2aec2]">
          The CLI on {device} is signed in. You can close this page.
        </p>
      </section>
    );
  }

  if (status === "denied") {
    return (
      <section className={cardClass} role="status">
        <h2>Denied.</h2>
        <p className="muted mt-[3px] text-[#a2aec2]">
          The CLI on {device} was not signed in. You can close this page.
        </p>
      </section>
    );
  }

  return (
    <section className={`${cardClass} grid gap-[18px]`} aria-label="CLI sign-in request">
      <div>
        <h2>Approve CLI sign-in?</h2>
        <p className="muted mt-[3px] text-[#a2aec2]">
          Check that this code matches the one in your terminal.
        </p>
      </div>
      <p
        aria-label="Code to compare"
        className="m-0 rounded-[10px] border border-[#303642] bg-[#0b0d10] py-4 text-center font-mono text-[32px] font-semibold tracking-[0.25em] text-[#edf1f7]"
      >
        {code}
      </p>
      {error && !request.data && (
        <p role="alert" className={errorClass}>
          {error.message}
        </p>
      )}
      {request.data && (
        <>
          <dl className="grid grid-cols-[max-content_1fr] gap-x-[18px] gap-y-[8px] text-[14px]">
            <dt className="text-[#8f9db2]">Device</dt>
            <dd className="m-0 text-[#d3dbe8]">{request.data.device_label}</dd>
            <dt className="text-[#8f9db2]">Requested</dt>
            <dd className="m-0 text-[#d3dbe8]">{formatRelative(request.data.created_at)}</dd>
            <dt className="text-[#8f9db2]">From address</dt>
            <dd className="m-0 font-mono text-[#d3dbe8]">{request.data.requested_from}</dd>
          </dl>
          {!request.data.same_address && (
            <p
              role="alert"
              className="rounded-[8px] border border-[#6b5a2a] bg-[#2a2415] px-4 py-[10px] text-[13px] text-[#f2d28f]"
            >
              This request came from a different network than this app. Only approve it if you just
              ran sparrow login yourself.
            </p>
          )}
          {decide.error && (
            <p role="alert" className={errorClass}>
              {decide.error.message}
            </p>
          )}
          <div className="flex gap-[8px]">
            <button
              className={primaryClass}
              disabled={decide.isPending}
              onClick={() => decide.mutate("approve")}
            >
              Approve
            </button>
            <button
              className={dangerClass}
              disabled={decide.isPending}
              onClick={() => decide.mutate("deny")}
            >
              Deny
            </button>
          </div>
        </>
      )}
    </section>
  );
}

/** Short steps for putting Super Sparrow on another computer. */
function NewComputerHelp() {
  return (
    <section className={cardClass} aria-label="Set up on a new computer">
      <h2>Set up on a new computer</h2>
      <ol className="mt-[10px] grid list-decimal gap-[6px] pl-[20px] text-[14px] text-[#a2aec2]">
        <li>Install the sparrow CLI on that computer.</li>
        <li>
          Run <code className="font-mono text-[#d3dbe8]">sparrow setup</code>. It signs in through
          this page and connects Claude Code and Codex.
        </li>
      </ol>
    </section>
  );
}
