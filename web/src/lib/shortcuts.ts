/** A keyboard shortcut as shown in the help dialog; `keys` are pressed one after another. */
export type ShortcutHelp = { keys: string[]; description: string };

export type ShortcutGroup = { title: string; shortcuts: ShortcutHelp[] };

/** The key that starts every "go to" sequence, such as G then I. */
export const goPrefix = "g";

/** Where each key after the go prefix leads. */
export const goDestinations = {
  i: "issues",
  y: "my-issues",
  a: "activity",
  m: "milestones",
  s: "people",
} as const;

export type GoDestination = (typeof goDestinations)[keyof typeof goDestinations];

export const shortcutGroups: ShortcutGroup[] = [
  {
    title: "Go to",
    shortcuts: [
      { keys: ["G", "I"], description: "Project issues" },
      { keys: ["G", "Y"], description: "My issues" },
      { keys: ["G", "A"], description: "Activity" },
      { keys: ["G", "M"], description: "Milestones" },
      { keys: ["G", "S"], description: "People and agents" },
      { keys: ["U"], description: "Up to the issue list" },
    ],
  },
  {
    title: "Create and search",
    shortcuts: [
      { keys: ["C"], description: "New issue" },
      { keys: ["/"], description: "Search issues" },
      { keys: ["Ctrl", "K"], description: "Open the navigation palette" },
    ],
  },
  {
    title: "Issue list",
    shortcuts: [
      { keys: ["J"], description: "Next issue" },
      { keys: ["K"], description: "Previous issue" },
      { keys: ["Enter"], description: "Open the focused issue" },
      { keys: ["V"], description: "Switch between list and board" },
    ],
  },
  {
    title: "Issue page",
    shortcuts: [
      { keys: ["1"], description: "Plan tab" },
      { keys: ["2"], description: "Review rounds tab" },
      { keys: ["3"], description: "Findings tab" },
      { keys: ["4"], description: "Activity tab" },
      { keys: ["5"], description: "Comments tab" },
      { keys: ["E"], description: "Edit the title" },
    ],
  },
  {
    title: "Anywhere",
    shortcuts: [
      { keys: ["?"], description: "Show this list" },
      { keys: ["Esc"], description: "Close a dialog or drawer" },
    ],
  },
];

/** Whether typing into this element should keep plain keys from acting as shortcuts. */
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;

  return ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName);
}

/** What a key press means after an optional pending go prefix. */
export type GoStep =
  { kind: "pending" } | { kind: "go"; destination: GoDestination } | { kind: "none" };

/** Advances the go sequence: the prefix waits for a destination key, and anything else cancels. */
export function nextGoStep(pending: boolean, key: string): GoStep {
  const lower = key.toLowerCase();
  if (!pending) return lower === goPrefix ? { kind: "pending" } : { kind: "none" };

  const destination = goDestinations[lower as keyof typeof goDestinations];
  return destination ? { kind: "go", destination } : { kind: "none" };
}
