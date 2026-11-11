import { describe, expect, test } from "bun:test";
import { formatCliCode, isCompleteCliCode, validateCliSearch } from "./cli-code";

describe("formatCliCode", () => {
  test("uppercases and groups the letters as they are typed", () => {
    expect(formatCliCode("b")).toBe("B");
    expect(formatCliCode("bcdf")).toBe("BCDF");
    expect(formatCliCode("bcdfg")).toBe("BCDF-G");
    expect(formatCliCode("bcdfghjk")).toBe("BCDF-GHJK");
  });

  test("ignores dashes, spaces and digits, so a pasted code is normalized", () => {
    expect(formatCliCode(" bcdf-ghjk ")).toBe("BCDF-GHJK");
    expect(formatCliCode("b1c2d3f-")).toBe("BCDF");
  });

  test("drops the dash again when the second group is deleted", () => {
    expect(formatCliCode("BCDF-")).toBe("BCDF");
  });

  test("keeps at most eight letters", () => {
    expect(formatCliCode("bcdfghjkmnp")).toBe("BCDF-GHJK");
  });
});

describe("isCompleteCliCode", () => {
  test("requires all eight letters", () => {
    expect(isCompleteCliCode("BCDF-GHJK")).toBe(true);
    expect(isCompleteCliCode("bcdfghjk")).toBe(true);
    expect(isCompleteCliCode("BCDF-GHJ")).toBe(false);
    expect(isCompleteCliCode("")).toBe(false);
  });
});

describe("validateCliSearch", () => {
  test("keeps a complete code in its display form", () => {
    expect(validateCliSearch({ code: "bcdfghjk" })).toEqual({ code: "BCDF-GHJK" });
  });

  test("drops missing, incomplete and non-string codes", () => {
    expect(validateCliSearch({})).toEqual({ code: undefined });
    expect(validateCliSearch({ code: "BCDF" })).toEqual({ code: undefined });
    expect(validateCliSearch({ code: 12345678 })).toEqual({ code: undefined });
  });
});
