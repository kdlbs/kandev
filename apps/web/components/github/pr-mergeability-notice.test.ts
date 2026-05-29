import { describe, it, expect } from "vitest";
import { describeMergeability, buildConflictResolutionMessage } from "./pr-mergeability-notice";
import type { MergeableState } from "@/lib/types/github";

function describe_({
  state,
  mergeable = false,
  isDraft = false,
  prState = "open",
}: {
  state: MergeableState | undefined;
  mergeable?: boolean;
  isDraft?: boolean;
  prState?: string;
}) {
  return describeMergeability({ state, mergeable, isDraft, prState });
}

describe("describeMergeability", () => {
  it("shows the conflict banner for a dirty PR", () => {
    expect(describe_({ state: "dirty" })).toEqual({ kind: "banner" });
  });

  it("shows a 'Blocked' chip for a blocked PR (even when GitHub reports mergeable)", () => {
    expect(describe_({ state: "blocked", mergeable: true })).toEqual({
      kind: "chip",
      label: "Blocked",
    });
  });

  it("shows a 'Behind base' chip for a behind PR", () => {
    expect(describe_({ state: "behind", mergeable: true })).toEqual({
      kind: "chip",
      label: "Behind base",
    });
  });

  it("shows nothing for clean / unstable / has_hooks", () => {
    expect(describe_({ state: "clean", mergeable: true })).toEqual({ kind: "none" });
    expect(describe_({ state: "unstable", mergeable: true })).toEqual({ kind: "none" });
    expect(describe_({ state: "has_hooks", mergeable: true })).toEqual({ kind: "none" });
  });

  it("falls back to generic text for unknown states that GitHub deems non-mergeable", () => {
    expect(describe_({ state: "unknown", mergeable: false })).toEqual({ kind: "text" });
    expect(describe_({ state: "", mergeable: false })).toEqual({ kind: "text" });
    expect(describe_({ state: undefined, mergeable: false })).toEqual({ kind: "text" });
  });

  it("shows nothing for unknown states GitHub still considers mergeable", () => {
    expect(describe_({ state: "unknown", mergeable: true })).toEqual({ kind: "none" });
  });

  it("suppresses every notice for draft PRs", () => {
    expect(describe_({ state: "dirty", isDraft: true })).toEqual({ kind: "none" });
  });

  it("suppresses every notice for non-open PRs", () => {
    expect(describe_({ state: "dirty", prState: "merged" })).toEqual({ kind: "none" });
    expect(describe_({ state: "dirty", prState: "closed" })).toEqual({ kind: "none" });
  });
});

describe("buildConflictResolutionMessage", () => {
  it("names the PR and both branches", () => {
    const msg = buildConflictResolutionMessage({
      prNumber: 1132,
      headBranch: "feature/x",
      baseBranch: "main",
    });
    expect(msg).toContain("#1132");
    expect(msg).toContain("`feature/x`");
    expect(msg).toContain("`main`");
    expect(msg.toLowerCase()).toContain("conflict");
  });
});
