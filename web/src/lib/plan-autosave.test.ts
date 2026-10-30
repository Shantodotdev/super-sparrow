import { describe, expect, test } from "bun:test";
import { ApiError } from "./api";
import { PlanAutosave, type PlanVersion } from "./plan-autosave";

const initial = { plan: "Original", revision: 3 };

/** Provides a controllable server response to exercise typing during a pending request. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((accept, decline) => {
    resolve = accept;
    reject = decline;
  });
  return { promise, resolve, reject };
}

describe("plan autosave", () => {
  test("debounces typing and sends the latest draft with its original revision", async () => {
    const writes: PlanVersion[] = [];
    const drafts: (PlanVersion | null)[] = [];
    const saver = new PlanAutosave({
      initial,
      delay: 10,
      persist: (draft) => drafts.push(draft),
      save: async (value) => {
        writes.push(value);
        return { ...value, revision: 4 };
      },
    });
    saver.edit("First");
    saver.edit("Latest");
    await Bun.sleep(40);
    expect(writes).toEqual([{ plan: "Latest", revision: 3 }]);
    expect(saver.state.dirty).toBe(false);
    expect(drafts.at(-1)).toBeNull();
  });

  test("serializes writes and preserves edits typed during an in-flight save", async () => {
    const first = deferred<PlanVersion>();
    const writes: PlanVersion[] = [];
    const saver = new PlanAutosave({
      initial,
      delay: 100000,
      persist: () => {},
      save: (value) => {
        writes.push(value);
        return writes.length === 1 ? first.promise : Promise.resolve({ ...value, revision: 5 });
      },
    });
    saver.edit("First");
    const pending = saver.flush();
    saver.edit("Second");
    await saver.flush();
    expect(writes).toHaveLength(1);
    first.resolve({ plan: "First", revision: 4 });
    await pending;
    expect(saver.state.plan).toBe("Second");
    expect(saver.state.dirty).toBe(true);
    await saver.flush();
    expect(writes[1]).toEqual({ plan: "Second", revision: 4 });
    expect(saver.state.dirty).toBe(false);
  });

  test("keeps a revert recoverable while an earlier save is still running", async () => {
    const first = deferred<PlanVersion>();
    const drafts: (PlanVersion | null)[] = [];
    let writes = 0;
    const saver = new PlanAutosave({
      initial,
      delay: 100000,
      persist: (draft) => drafts.push(draft),
      save: (value) =>
        ++writes === 1 ? first.promise : Promise.resolve({ ...value, revision: 5 }),
    });
    saver.edit("Temporary edit");
    const pending = saver.flush();
    saver.edit(initial.plan);
    expect(drafts.at(-1)).toEqual(initial);
    expect(saver.state.saving).toBe(true);
    first.resolve({ plan: "Temporary edit", revision: 4 });
    await pending;
    expect(saver.state.dirty).toBe(true);
    await saver.flush();
    expect(saver.state.plan).toBe(initial.plan);
    expect(drafts.at(-1)).toBeNull();
  });

  test("stops on conflicts, keeps the draft, and refuses to adopt a newer revision", async () => {
    let writes = 0;
    const drafts: (PlanVersion | null)[] = [];
    const saver = new PlanAutosave({
      initial,
      persist: (value) => {
        drafts.push(value);
      },
      save: async () => {
        writes++;
        throw new ApiError(409, "Plan changed elsewhere");
      },
    });
    saver.edit("My draft");
    await saver.flush();
    expect(saver.receive({ plan: "Someone else's draft", revision: 4 })).toBe(false);
    saver.edit("My draft continued");
    await saver.flush();
    expect(writes).toBe(1);
    expect(drafts.at(-1)).toEqual({ plan: "My draft continued", revision: 3 });
    expect(saver.state.error?.message).toBe("Plan changed elsewhere");
    saver.reset({ plan: "Someone else's draft", revision: 4 });
    expect(saver.state.dirty).toBe(false);
    expect(saver.state.error).toBeNull();
  });

  test("accepts remote changes only while clean and ignores older revisions", () => {
    const saver = new PlanAutosave({ initial, persist: () => {}, save: async (value) => value });
    expect(saver.receive({ plan: "Remote", revision: 4 })).toBe(true);
    expect(saver.receive(initial)).toBe(false);
    saver.edit("Local");
    expect(saver.receive({ plan: "Remote again", revision: 5 })).toBe(false);
    saver.reset({ plan: "Remote again", revision: 5 });
    expect(saver.state.plan).toBe("Remote again");
  });

  test("recovers a local draft using its saved revision rather than the current remote revision", async () => {
    const writes: PlanVersion[] = [];
    const saver = new PlanAutosave({
      initial: { plan: "Remote", revision: 8 },
      draft: { plan: "Recovered", revision: 6 },
      persist: () => {},
      save: async (value) => {
        writes.push(value);
        throw new ApiError(409, "Conflict");
      },
    });
    await saver.flush();
    expect(writes).toEqual([{ plan: "Recovered", revision: 6 }]);
    expect(saver.state.plan).toBe("Recovered");
  });

  test("retains network failures and retries with the unchanged base revision", async () => {
    let attempt = 0;
    const writes: PlanVersion[] = [];
    const saver = new PlanAutosave({
      initial,
      persist: () => {},
      save: async (value) => {
        writes.push(value);
        if (++attempt === 1) throw new Error("Offline");
        return { ...value, revision: 4 };
      },
    });
    saver.edit("Offline draft");
    await saver.flush();
    expect(saver.state.dirty).toBe(true);
    expect(saver.state.error?.message).toBe("Offline");
    await saver.flush();
    expect(writes[1]).toEqual({ plan: "Offline draft", revision: 3 });
    expect(saver.state.dirty).toBe(false);
    expect(saver.state.error).toBeNull();
  });

  test("does not write unchanged content or a draft reverted before debounce", async () => {
    let writes = 0;
    const saver = new PlanAutosave({
      initial,
      persist: () => {},
      save: async (value) => {
        writes++;
        return value;
      },
    });
    saver.edit("Changed");
    saver.edit(initial.plan);
    await saver.flush();
    expect(writes).toBe(0);
  });
});
