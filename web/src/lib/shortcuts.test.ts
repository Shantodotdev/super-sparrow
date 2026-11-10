import { describe, expect, test } from "bun:test";
import {
  defaultBindings,
  matchesChord,
  resolveBindings,
  recordChord,
  type ShortcutKeyEvent,
} from "./shortcuts";

const press = (code: string, modifiers: Partial<ShortcutKeyEvent> = {}): ShortcutKeyEvent => ({
  key: code.startsWith("Key") ? code.slice(3).toLowerCase() : code,
  code,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...modifiers,
});

describe("personal keyboard bindings", () => {
  test("uses Meta+Shift+Q to open capture and Ctrl+Q only to close it", () => {
    expect(
      matchesChord(press("KeyQ", { metaKey: true, shiftKey: true }), defaultBindings.quickCapture),
    ).toBe(true);
    expect(matchesChord(press("KeyQ", { ctrlKey: true }), defaultBindings.quickCapture)).toBe(
      false,
    );
    expect(matchesChord(press("KeyQ", { ctrlKey: true }), defaultBindings.closeCapture)).toBe(true);
  });
  test("requires exact modifiers and supports both Mod accelerators", () => {
    expect(matchesChord(press("KeyK", { ctrlKey: true }), "Mod+K")).toBe(true);
    expect(matchesChord(press("KeyK", { metaKey: true }), "Mod+K")).toBe(true);
    expect(matchesChord(press("KeyK", { ctrlKey: true, metaKey: true }), "Mod+K")).toBe(false);
    expect(matchesChord(press("KeyK", { ctrlKey: true, shiftKey: true }), "Mod+K")).toBe(false);
  });
  test("matches physical punctuation and disables an empty binding", () => {
    expect(matchesChord(press("Slash", { key: "?", shiftKey: true }), "Shift+Slash")).toBe(true);
    expect(matchesChord(press("KeyQ"), "")).toBe(false);
    expect(matchesChord(press("KeyG"), "G I")).toBe(false);
  });
  test("records modifiers in canonical order and ignores cancellation and modifier-only presses", () => {
    expect(recordChord(press("KeyQ", { metaKey: true, shiftKey: true }))).toBe("Meta+Shift+Q");
    expect(recordChord(press("KeyN", { ctrlKey: true, altKey: true }))).toBe("Ctrl+Alt+N");
    expect(recordChord(press("Escape"))).toBeNull();
    expect(recordChord(press("Tab"))).toBeNull();
    expect(recordChord(press("MetaLeft", { key: "Meta", metaKey: true }))).toBeNull();
  });
  test("keeps defaults independent across accounts and preserves disabled overrides", () => {
    const personal = resolveBindings({ quickCapture: "Ctrl+Alt+K", newIssue: "", unknown: "X" });
    expect(personal.quickCapture).toBe("Ctrl+Alt+K");
    expect(personal.newIssue).toBe("");
    expect(resolveBindings({}).quickCapture).toBe("Meta+Shift+Q");
    expect("unknown" in personal).toBe(false);
  });
});
