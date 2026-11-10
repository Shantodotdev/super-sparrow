import { describe, expect, test } from "bun:test";
import { isQuickCaptureKey, nextGoStep } from "./shortcuts";

describe("nextGoStep", () => {
  test("starts a sequence on the go prefix only", () => {
    expect(nextGoStep(false, "g")).toEqual({ kind: "pending" });
    expect(nextGoStep(false, "G")).toEqual({ kind: "pending" });
    expect(nextGoStep(false, "i")).toEqual({ kind: "none" });
  });
  test("resolves a destination after the prefix", () => {
    expect(nextGoStep(true, "i")).toEqual({ kind: "go", destination: "issues" });
    expect(nextGoStep(true, "Y")).toEqual({ kind: "go", destination: "my-issues" });
    expect(nextGoStep(true, "m")).toEqual({ kind: "go", destination: "milestones" });
  });
  test("cancels on an unknown key after the prefix", () => {
    expect(nextGoStep(true, "z")).toEqual({ kind: "none" });
    expect(nextGoStep(true, "g")).toEqual({ kind: "none" });
  });
});

describe("isQuickCaptureKey", () => {
  const press = (key: string, modifiers: Partial<KeyboardEvent> = {}) => ({
    key,
    code: `Key${key.toUpperCase()}`,
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    ...modifiers,
  });

  test("matches Ctrl+Q in either case", () => {
    expect(isQuickCaptureKey(press("q", { ctrlKey: true }))).toBe(true);
    expect(isQuickCaptureKey(press("Q", { ctrlKey: true }))).toBe(true);
  });

  test("leaves plain Q, Cmd+Q, and other combinations alone", () => {
    expect(isQuickCaptureKey(press("q"))).toBe(false);
    expect(isQuickCaptureKey(press("q", { metaKey: true }))).toBe(false);
    expect(isQuickCaptureKey(press("q", { ctrlKey: true, shiftKey: true }))).toBe(false);
    expect(isQuickCaptureKey(press("w", { ctrlKey: true }))).toBe(false);
  });
});
