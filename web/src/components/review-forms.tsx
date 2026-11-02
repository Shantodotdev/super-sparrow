import { useState, type FormEvent } from "react";
import { severityLabel, findingSeverities } from "../lib/finding-status";
import type { Finding, Issue, ReviewRound, FindingSeverity } from "../lib/types";
import { useCreateFinding, useFinishReview, useStartReview } from "../queries/reviews";
import { Input, Textarea } from "./input";
import { Select } from "./select";

export const reviewButton =
  "inline-flex items-center justify-center gap-2 rounded-lg bg-[#426fff] px-4 py-2.5 text-[13px] font-medium text-white transition-colors hover:bg-[#537dff] disabled:cursor-default disabled:opacity-50";
export const reviewSecondaryButton =
  "inline-flex items-center justify-center gap-2 rounded-lg border border-[#303641] px-4 py-2.5 text-[13px] text-[#c4cfdf] transition-colors hover:bg-[#20242b] disabled:cursor-default disabled:opacity-50";
const fieldStyle = "flex flex-col gap-2 text-[13px] text-[#c4cfdf]";
const inputStyle = "w-full rounded-lg px-3 py-2.5";

/** Creates the round record without automatically running an agent. */
export function StartReviewForm({
  issue,
  onCreated,
}: {
  issue: Issue;
  onCreated: (round: ReviewRound) => void;
}) {
  const [coverage, setCoverage] = useState("");
  const start = useStartReview(issue.key);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (start.isPending) return;
    try {
      const { round } = await start.mutateAsync(coverage.trim());
      onCreated(round);
    } catch {
      /* The mutation error is shown with the draft intact. */
    }
  };

  return (
    <form onSubmit={submit} className="space-y-6">
      <h2 className="text-[23px] font-semibold">Start review round</h2>
      <p className="leading-relaxed text-[#97a4b7]">
        Create the next review round for {issue.key}. You or your connected agent can add findings
        as you review.
      </p>
      <label className={fieldStyle}>
        Plan coverage{" "}
        <span className="text-[12px] text-[#8290a4]">
          Optional — which parts of the plan will you review?
        </span>
        <Textarea
          autoFocus
          aria-label="Plan coverage"
          rows={4}
          maxLength={2000}
          value={coverage}
          onChange={(event) => setCoverage(event.target.value)}
        />
      </label>
      {start.error && (
        <p role="alert" className="text-[13px] text-[#f2a9ae]">
          {start.error.message}
        </p>
      )}
      <button type="submit" disabled={start.isPending} className={reviewButton}>
        {start.isPending ? "Starting…" : "Start review round"}
      </button>
    </form>
  );
}

/** Closes the round with a summary; pending findings remain actionable. */
export function FinishReviewForm({
  issue,
  round,
  onFinished,
}: {
  issue: Issue;
  round: ReviewRound;
  onFinished: () => void;
}) {
  const [summary, setSummary] = useState("");
  const finish = useFinishReview(issue.key, round.number);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (finish.isPending) return;
    try {
      await finish.mutateAsync(summary.trim());
      onFinished();
    } catch {
      /* The mutation error is shown with the draft intact. */
    }
  };

  return (
    <form onSubmit={submit} className="space-y-6">
      <h2 className="text-[23px] font-semibold">Finish round {round.number}</h2>
      <p className="leading-relaxed text-[#97a4b7]">
        This closes the round to new findings. Existing findings can still be discussed, fixed, and
        verified.
      </p>
      <label className={fieldStyle}>
        Summary
        <Textarea
          autoFocus
          aria-label="Review summary"
          rows={6}
          maxLength={100000}
          value={summary}
          onChange={(event) => setSummary(event.target.value)}
        />
      </label>
      {finish.error && (
        <p role="alert" className="text-[13px] text-[#f2a9ae]">
          {finish.error.message}
        </p>
      )}
      <button type="submit" disabled={finish.isPending} className={reviewButton}>
        {finish.isPending ? "Finishing…" : "Finish review round"}
      </button>
    </form>
  );
}

/** Adds a finding to a specific active round, keeping optional evidence out of the main form. */
export function AddFindingForm({
  issue,
  round,
  onCreated,
}: {
  issue: Issue;
  round: ReviewRound;
  onCreated: (finding: Finding) => void;
}) {
  const [severity, setSeverity] = useState<FindingSeverity>("should_fix");
  const create = useCreateFinding(issue.key, round.number);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (create.isPending) return;
    const data = new FormData(event.currentTarget);
    const field = (name: string) => String(data.get(name) ?? "").trim();
    try {
      const { finding } = await create.mutateAsync({
        severity,
        title: field("title"),
        problem: field("problem"),
        fix: field("fix"),
        evidence: field("evidence"),
        file_path: field("file"),
        ...(field("line") ? { line: Number(field("line")) } : {}),
      });
      onCreated(finding);
    } catch {
      /* The mutation error is shown with the draft intact. */
    }
  };

  return (
    <form onSubmit={submit} className="space-y-6">
      <div>
        <h2 className="text-[23px] font-semibold">Add finding</h2>
        <p className="mt-2 text-[13px] text-[#97a4b7]">
          {issue.key} · Round {round.number}
        </p>
      </div>
      <label className={fieldStyle}>
        Title
        <Input
          autoFocus
          name="title"
          aria-label="Finding title"
          required
          maxLength={200}
          className={inputStyle}
        />
      </label>
      <Select
        label="Severity"
        value={severity}
        options={findingSeverities.map((value) => ({ value, label: severityLabel[value] }))}
        onChange={(value) => setSeverity(value as FindingSeverity)}
      />
      <label className={fieldStyle}>
        Problem
        <Textarea
          name="problem"
          aria-label="Finding problem"
          required
          rows={3}
          maxLength={100000}
        />
      </label>
      <label className={fieldStyle}>
        Suggested fix
        <Textarea name="fix" aria-label="Suggested fix" required rows={3} maxLength={100000} />
      </label>
      <details className="text-[13px] text-[#a9b8ca]">
        <summary className="cursor-pointer py-1">
          Evidence and location <span className="text-[#8290a4]">(optional)</span>
        </summary>
        <div className="mt-4 space-y-4">
          <label className={fieldStyle}>
            Evidence
            <Textarea name="evidence" aria-label="Finding evidence" rows={3} maxLength={100000} />
          </label>
          <div className="grid grid-cols-[minmax(0,1fr)_90px] gap-3">
            <label className={fieldStyle}>
              File path
              <Input name="file" maxLength={1000} className={inputStyle} />
            </label>
            <label className={fieldStyle}>
              Line
              <Input name="line" type="number" min={1} step={1} className={inputStyle} />
            </label>
          </div>
        </div>
      </details>
      {create.error && (
        <p role="alert" className="text-[13px] text-[#f2a9ae]">
          {create.error.message}
        </p>
      )}
      <button type="submit" disabled={create.isPending} className={reviewButton}>
        {create.isPending ? "Adding…" : "Add finding"}
      </button>
    </form>
  );
}
