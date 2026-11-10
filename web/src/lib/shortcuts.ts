import definitions from "../../../server/internal/keyboard/definitions.json";

export const shortcutDefinitions = definitions;
export type ShortcutID = keyof typeof definitions;
export type ShortcutBindings = Record<ShortcutID, string>;
export const defaultBindings = Object.fromEntries(
  Object.entries(definitions).map(([id, definition]) => [id, definition.binding]),
) as ShortcutBindings;

/** Resolves disabled and customized bindings without modifying the shared defaults. */
export function resolveBindings(overrides: Record<string, string>): ShortcutBindings {
  return {
    ...defaultBindings,
    ...Object.fromEntries(Object.entries(overrides).filter(([id]) => id in definitions)),
  };
}

export type ShortcutKeyEvent = Pick<
  KeyboardEvent,
  "key" | "code" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
>;

/** Converts a physical key into the same canonical key name used by the native shortcut API. */
export function shortcutKey(event: ShortcutKeyEvent): string {
  if (/^Key[A-Z]$/.test(event.code)) return event.code.slice(3);
  if (/^Digit[0-9]$/.test(event.code)) return event.code.slice(5);
  if (["Shift", "Control", "Alt", "Meta", "AltGraph", "Dead", "Unidentified"].includes(event.key))
    return "";
  const punctuation: Record<string, string> = {
    "?": "Slash",
    "/": "Slash",
    ",": "Comma",
    ".": "Period",
    ";": "Semicolon",
    '"': "Quote",
    "\'": "Quote",
    "`": "Backquote",
    "[": "BracketLeft",
    "]": "BracketRight",
    "\\": "Backslash",
    "-": "Minus",
    "=": "Equal",
    "+": "Equal",
  };
  return (
    event.code ||
    punctuation[event.key] ||
    (event.key === " " ? "Space" : event.key.length === 1 ? event.key.toUpperCase() : event.key)
  );
}

/** Captures a physical key chord while leaving Escape and Tab for cancellation and focus. */
export function recordChord(event: ShortcutKeyEvent): string | null {
  const key = shortcutKey(event);
  if (!key || ["Escape", "Tab"].includes(key)) return null;
  return [
    event.ctrlKey && "Ctrl",
    event.altKey && "Alt",
    event.metaKey && "Meta",
    event.shiftKey && "Shift",
    key,
  ]
    .filter(Boolean)
    .join("+");
}

/** Matches exactly one chord, including modifiers; Mod accepts either Ctrl or Meta, but not both. */
export function matchesChord(event: ShortcutKeyEvent, chord: string): boolean {
  if (!chord || chord.includes(" ")) return false;
  const parts = chord.split("+");
  const key = parts.pop();
  const modifiers = new Set(parts);
  const mod = modifiers.has("Mod");
  return (
    shortcutKey(event) === key &&
    (mod
      ? event.ctrlKey !== event.metaKey
      : event.ctrlKey === modifiers.has("Ctrl") && event.metaKey === modifiers.has("Meta")) &&
    event.altKey === modifiers.has("Alt") &&
    event.shiftKey === modifiers.has("Shift")
  );
}

/** Whether a target is an editor or keyboard-operated input widget. */
export function isTypingTarget(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      !!target.closest(
        "input, textarea, select, [contenteditable=true], [role=combobox], [role=listbox]",
      ))
  );
}

/** Displays sequences distinctly from simultaneous chords. Meta is Command on macOS, Super on Linux, and Windows on Windows. */
export function formatBinding(binding: string): string {
  return binding ? binding.split(" ").join(" then ") : "Disabled";
}
