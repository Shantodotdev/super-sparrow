import { describe, expect, test } from "bun:test";
import { nextGoStep } from "./shortcuts";

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
