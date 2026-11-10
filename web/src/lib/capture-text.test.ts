import { describe, expect, test } from "bun:test";
import { splitCapture } from "./capture-text";

describe("splitCapture", () => {
  test("uses the first line as the title and the rest as the body", () => {
    expect(splitCapture("Header overlaps the cart\n\nSteps:\n- open checkout")).toEqual({
      title: "Header overlaps the cart",
      body: "Steps:\n- open checkout",
    });
  });

  test("strips Markdown from the title", () => {
    expect(splitCapture("## **Broken** [login](https://x.test) button").title).toBe(
      "Broken login button",
    );
    expect(splitCapture("- [ ] Fix the `save` flow").title).toBe("Fix the save flow");
  });

  test("skips images before the first words and keeps them in the body", () => {
    expect(splitCapture("![Screenshot](/api/attachments/1)\n\nButton is cut off")).toEqual({
      title: "Button is cut off",
      body: "![Screenshot](/api/attachments/1)",
    });
  });

  test("titles a description of only images Screenshot", () => {
    expect(splitCapture("![Screenshot](/api/attachments/1)")).toEqual({
      title: "Screenshot",
      body: "![Screenshot](/api/attachments/1)",
    });
  });

  test("leaves an empty description empty", () => {
    expect(splitCapture("  \n ")).toEqual({ title: "", body: "" });
  });

  test("shortens a long first line to the accepted title length", () => {
    const title = splitCapture(`${"word ".repeat(60)}end`).title;
    expect(title.length).toBeLessThanOrEqual(200);
    expect(title.endsWith("…")).toBe(true);
  });
});
