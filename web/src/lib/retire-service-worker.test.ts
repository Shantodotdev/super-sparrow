import { expect, test } from "bun:test";
import { retireServiceWorker } from "./retire-service-worker";

test("unregisters the former app worker in any lifecycle state", async () => {
  const removed: number[] = [];
  const registrations = ["active", "waiting", "installing"].map((state, index) => ({
    active: null,
    waiting: null,
    installing: null,
    [state]: { scriptURL: "https://sparrow.example/sw.js" },
    unregister: async () => {
      removed.push(index);
      return true;
    },
  }));

  await retireServiceWorker(
    { getRegistrations: async () => registrations },
    "https://sparrow.example",
  );

  expect(removed).toEqual([0, 1, 2]);
});

test("preserves unrelated registrations and registrations with no worker", async () => {
  let removed = false;
  const registrations = [
    { scriptURL: "https://sparrow.example/another-worker.js" },
    { scriptURL: "https://another.example/sw.js" },
    null,
  ].map((active) => ({
    active,
    waiting: null,
    installing: null,
    unregister: async () => {
      removed = true;
      return true;
    },
  }));

  await retireServiceWorker(
    { getRegistrations: async () => registrations },
    "https://sparrow.example",
  );

  expect(removed).toBe(false);
});

test("accepts browsers with no previous registration", async () => {
  await expect(
    retireServiceWorker({ getRegistrations: async () => [] }, "https://sparrow.example"),
  ).resolves.toBeUndefined();
});
