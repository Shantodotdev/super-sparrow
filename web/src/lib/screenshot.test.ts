import { describe, expect, test } from "bun:test";
import { dragRegion } from "./screenshot";

describe("dragRegion", () => {
  const bounds = { width: 2000, height: 1000 };

  test("maps a drag on a half-size image to image pixels in any direction", () => {
    expect(dragRegion({ x: 300, y: 200 }, { x: 100, y: 50 }, 2, bounds)).toEqual({
      x: 200,
      y: 100,
      width: 400,
      height: 300,
    });
  });

  test("keeps the region inside the image", () => {
    expect(dragRegion({ x: -20, y: 400 }, { x: 1500, y: 900 }, 2, bounds)).toEqual({
      x: 0,
      y: 800,
      width: 2000,
      height: 200,
    });
  });

  test("ignores a click or a tiny drag", () => {
    expect(dragRegion({ x: 10, y: 10 }, { x: 12, y: 40 }, 1, bounds)).toBeNull();
  });
});
