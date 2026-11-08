import { describe, expect, it } from "vitest";
import {
  addRef,
  depErrorKey,
  parseRefInput,
  refKey,
  removeRef,
  sameRefs,
} from "./merge-dependencies";

describe("parseRefInput", () => {
  it("treats a pipe as a change key and anything else as a task id", () => {
    expect(parseRefInput(" github|h|r|7 ")).toEqual({ change: "github|h|r|7" });
    expect(parseRefInput(" t2 ")).toEqual({ task: "t2" });
    expect(parseRefInput("   ")).toBeNull();
  });
});

describe("addRef", () => {
  it("appends a new reference", () => {
    expect(addRef([{ task: "a" }], "b", "self")).toEqual({
      ok: true,
      refs: [{ task: "a" }, { task: "b" }],
    });
  });

  it("refuses empty, duplicate and self", () => {
    expect(addRef([], " ", "self")).toEqual({ ok: false, reason: "empty" });
    expect(addRef([{ task: "a" }], "a", "self")).toEqual({ ok: false, reason: "duplicate" });
    expect(addRef([{ change: "g|h|r|1" }], "g|h|r|1", "self")).toEqual({
      ok: false,
      reason: "duplicate",
    });
    expect(addRef([], "self", "self")).toEqual({ ok: false, reason: "self" });
  });

  it("keeps a task and a change with the same text apart", () => {
    expect(refKey({ task: "x" })).not.toBe(refKey({ change: "x" }));
  });
});

describe("removeRef and sameRefs", () => {
  it("removes by identity and compares as sets", () => {
    expect(removeRef([{ task: "a" }, { task: "b" }], { task: "a" })).toEqual([{ task: "b" }]);
    expect(sameRefs([{ task: "a" }, { task: "b" }], [{ task: "b" }, { task: "a" }])).toBe(true);
    expect(sameRefs([{ task: "a" }], [{ task: "b" }])).toBe(false);
    expect(sameRefs([{ task: "a" }], [])).toBe(false);
  });
});

describe("depErrorKey", () => {
  it("maps closed codes and falls back to a generic failure", () => {
    expect(depErrorKey("dependency_duplicate")).toBe("coordinator:mergeDepDuplicate");
    expect(depErrorKey("revision_conflict")).toBe("coordinator:mergeDepConflict");
    expect(depErrorKey("nope")).toBe("coordinator:mergeDepSaveFailed");
    expect(depErrorKey(null)).toBe("coordinator:mergeDepSaveFailed");
  });
});
