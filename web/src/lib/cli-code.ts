const codeLetters = 8;
const groupLength = 4;

/** Formats typed or pasted text as a CLI sign-in code such as "BCDF-GHJK": letters only, uppercase, at most eight. */
export function formatCliCode(input: string): string {
  const letters = input
    .replace(/[^a-z]/gi, "")
    .toUpperCase()
    .slice(0, codeLetters);

  if (letters.length <= groupLength) return letters;

  return `${letters.slice(0, groupLength)}-${letters.slice(groupLength)}`;
}

/** Whether the text holds all eight letters of a CLI sign-in code. */
export function isCompleteCliCode(input: string): boolean {
  return formatCliCode(input).replace("-", "").length === codeLetters;
}

/** The complete code a page was opened with; missing or incomplete codes are dropped so the form asks for it. */
export type CliSearch = { code?: string };

export function validateCliSearch(search: Record<string, unknown>): CliSearch {
  const code = typeof search.code === "string" ? formatCliCode(search.code) : "";

  return { code: isCompleteCliCode(code) ? code : undefined };
}
