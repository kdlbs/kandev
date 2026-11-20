import { describe, expect, it } from "vitest";
import { isMRDraft, isMergeStatusTransient } from "./gitlab-merge-state";

describe("isMRDraft", () => {
  it.each([
    [{ draft: true }, true],
    [{ detailed_merge_status: "draft_status" }, true],
    [{ draft: true, detailed_merge_status: "mergeable" }, true],
    [{ draft: false, detailed_merge_status: "mergeable" }, false],
    [{}, false],
  ])("%j -> %s", (mr, want) => {
    expect(isMRDraft(mr)).toBe(want);
  });
});

describe("isMergeStatusTransient", () => {
  it.each([
    ["checking", true],
    ["unchecked", true],
    ["preparing", true],
    ["mergeable", false],
    ["draft_status", false],
    ["", false],
    [undefined, false],
  ])("%s -> %s", (status, want) => {
    expect(isMergeStatusTransient(status)).toBe(want);
  });
});
