import type { PlanVersion } from "./plan-autosave";

/** Reads only valid local editor drafts; unavailable browser storage never prevents editing. */
export function readPlanDraft(key: string): PlanVersion | undefined {
  try {
    const value = JSON.parse(sessionStorage.getItem(key) ?? "null");
    if (value && typeof value.plan === "string" && Number.isInteger(value.revision)) return value;
  } catch {
    return undefined;
  }
}

/** Keeps unsaved Markdown across reloads without treating browser storage as server state. */
export function storePlanDraft(key: string, draft: PlanVersion | null): void {
  try {
    if (draft) sessionStorage.setItem(key, JSON.stringify(draft));
    else sessionStorage.removeItem(key);
  } catch {
    // Private browsing or a full storage quota must not interrupt editing or saving.
  }
}
