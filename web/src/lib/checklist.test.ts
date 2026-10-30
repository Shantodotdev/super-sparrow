import { describe, expect, test } from "bun:test";
import { checklistItems, setChecked } from "./checklist";

const plan = [
  "## Steps",
  "- [ ] Migration",
  "1. [x] Store",
  "```",
  "- [ ] An example, not a task",
  "```",
  "* [ ] Tests",
].join("\n");

describe("checklist", () => {
  test("numbers items in order and skips fenced examples", () => {
    expect(checklistItems(plan)).toEqual([
      { index: 0, text: "Migration", checked: false },
      { index: 1, text: "Store", checked: true },
      { index: 2, text: "Tests", checked: false },
    ]);
  });

  test("ticks one item and leaves every other line as it was", () => {
    const ticked = setChecked(plan, 2, true);

    expect(ticked).toBe(plan.replace("* [ ] Tests", "* [x] Tests"));
    expect(setChecked(ticked, 1, false)).toBe(ticked.replace("1. [x] Store", "1. [ ] Store"));
  });
});
