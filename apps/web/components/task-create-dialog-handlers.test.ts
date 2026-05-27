import { describe, it, expect, vi } from "vitest";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import {
  makeRemoteRowAdd,
  makeRemoteRowChange,
  makeRemoteRowRemove,
} from "./task-create-dialog-handlers";

const URL_A = "https://github.com/a/b";
const URL_C = "https://github.com/c/d";

// Tiny helper that mimics React's setState callback contract: hold a value,
// expose a setter that applies an updater fn (the only form these helpers use).
function makeStateRef(initial: TaskRemoteRepoRow[]) {
  let value = initial;
  const setter = vi.fn(
    (updater: TaskRemoteRepoRow[] | ((prev: TaskRemoteRepoRow[]) => TaskRemoteRepoRow[])) => {
      value = typeof updater === "function" ? updater(value) : updater;
    },
  );
  return {
    setter: setter as unknown as React.Dispatch<React.SetStateAction<TaskRemoteRepoRow[]>>,
    get value() {
      return value;
    },
  };
}

describe("makeRemoteRowChange", () => {
  it("merges the patch into the matching row only", () => {
    const ref = makeStateRef([
      { key: "remote-0", url: URL_A, branch: "", source: "paste" },
      { key: "remote-1", url: "", branch: "", source: "paste" },
    ]);
    const change = makeRemoteRowChange(ref.setter);
    change("remote-1", { url: URL_C, source: "picker" });
    expect(ref.value[1].url).toBe(URL_C);
    expect(ref.value[1].source).toBe("picker");
    // The other row is untouched.
    expect(ref.value[0].url).toBe(URL_A);
  });

  it("does nothing when no row matches the key", () => {
    const ref = makeStateRef([{ key: "remote-0", url: URL_A, branch: "", source: "paste" }]);
    const change = makeRemoteRowChange(ref.setter);
    change("nope", { url: "x" });
    expect(ref.value).toEqual([{ key: "remote-0", url: URL_A, branch: "", source: "paste" }]);
  });
});

describe("makeRemoteRowAdd", () => {
  it("appends a new empty row with a stable key", () => {
    const ref = makeStateRef([{ key: "remote-0", url: URL_A, branch: "", source: "paste" }]);
    const add = makeRemoteRowAdd(ref.setter);
    add();
    expect(ref.value).toHaveLength(2);
    expect(ref.value[1]).toMatchObject({ url: "", branch: "", source: "paste" });
    expect(ref.value[1].key).toBeTruthy();
    expect(ref.value[1].key).not.toBe("remote-0");
  });

  it("generates unique keys across multiple adds", () => {
    const ref = makeStateRef([] as TaskRemoteRepoRow[]);
    const add = makeRemoteRowAdd(ref.setter);
    add();
    add();
    add();
    const keys = ref.value.map((r) => r.key);
    expect(new Set(keys).size).toBe(3);
  });
});

describe("makeRemoteRowRemove", () => {
  it("drops the row with the given key", () => {
    const ref = makeStateRef([
      { key: "remote-0", url: "a", branch: "", source: "paste" },
      { key: "remote-1", url: "b", branch: "", source: "paste" },
      { key: "remote-2", url: "c", branch: "", source: "paste" },
    ]);
    const remove = makeRemoteRowRemove(ref.setter);
    remove("remote-1");
    expect(ref.value.map((r) => r.key)).toEqual(["remote-0", "remote-2"]);
  });

  it("is a no-op when the key isn't present", () => {
    const ref = makeStateRef([{ key: "remote-0", url: "a", branch: "", source: "paste" }]);
    const remove = makeRemoteRowRemove(ref.setter);
    remove("nope");
    expect(ref.value).toEqual([{ key: "remote-0", url: "a", branch: "", source: "paste" }]);
  });
});
