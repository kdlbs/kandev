import { describe, it, expect } from "vitest";
import {
  computeBranchPrefix,
  computeBranchTooltip,
  computeBranchDisabledReason,
} from "./task-create-dialog-branch-utils";

describe("computeBranchPrefix", () => {
  it("returns empty when no branch is picked yet", () => {
    expect(
      computeBranchPrefix({
        isLocalExecutor: true,
        rowBranch: "",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("");
  });

  it("returns 'current: ' for local executor when row branch matches workspace current", () => {
    expect(
      computeBranchPrefix({
        isLocalExecutor: true,
        rowBranch: "main",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("current: ");
  });

  it("returns 'current: ' on detached HEAD when the picked value is the same short SHA", () => {
    // Backend returns a short SHA in `current_branch` when the repo is on
    // detached HEAD; the autoselect seeds row.branch with that same value
    // so the chip can read "current: <sha>" instead of falling through to
    // "will switch to: master".
    expect(
      computeBranchPrefix({
        isLocalExecutor: true,
        rowBranch: "4fbc5d7",
        currentLocalBranch: "4fbc5d7",
        freshBranchEnabled: false,
      }),
    ).toBe("current: ");
  });

  it("returns 'will switch to: ' for local executor when row branch differs from current", () => {
    expect(
      computeBranchPrefix({
        isLocalExecutor: true,
        rowBranch: "develop",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("will switch to: ");
  });

  it("treats unknown current branch as 'will switch to' on local executor", () => {
    // Local-status fetch error leaves currentLocalBranch="". With no baseline
    // we can't claim the picked branch is "current", so fall through to the
    // destructive label.
    expect(
      computeBranchPrefix({
        isLocalExecutor: true,
        rowBranch: "main",
        currentLocalBranch: "",
        freshBranchEnabled: false,
      }),
    ).toBe("will switch to: ");
  });

  it("returns 'from: ' for non-local executors regardless of current branch", () => {
    expect(
      computeBranchPrefix({
        isLocalExecutor: false,
        rowBranch: "main",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("from: ");
    expect(
      computeBranchPrefix({
        isLocalExecutor: false,
        rowBranch: "develop",
        currentLocalBranch: "",
        freshBranchEnabled: false,
      }),
    ).toBe("from: ");
  });

  it("returns 'from: ' when fork-a-new-branch is enabled, even on local executor", () => {
    expect(
      computeBranchPrefix({
        isLocalExecutor: true,
        rowBranch: "main",
        currentLocalBranch: "main",
        freshBranchEnabled: true,
      }),
    ).toBe("from: ");
    expect(
      computeBranchPrefix({
        isLocalExecutor: true,
        rowBranch: "develop",
        currentLocalBranch: "main",
        freshBranchEnabled: true,
      }),
    ).toBe("from: ");
  });
});

describe("computeBranchTooltip", () => {
  it('returns the "current" tooltip for "current: " prefix', () => {
    expect(computeBranchTooltip("current: ")).toContain("as-is");
  });

  it('returns the "switch" tooltip for "will switch to: " prefix', () => {
    expect(computeBranchTooltip("will switch to: ")).toContain("git checkout");
  });

  it('returns the "fork" tooltip for "from: " prefix', () => {
    expect(computeBranchTooltip("from: ")).toContain("forked");
  });

  it("returns a generic tooltip for empty or undefined prefix", () => {
    expect(computeBranchTooltip("")).toContain("Branch the agent will run against");
    expect(computeBranchTooltip(undefined)).toContain("Branch the agent will run against");
  });

  it("returns a generic tooltip for an unrecognized prefix", () => {
    // Defensive: unknown prefix shouldn't crash, just falls back.
    expect(computeBranchTooltip("foo: ")).toContain("Branch the agent will run against");
  });
});

describe("computeBranchDisabledReason", () => {
  it("returns the lock reason when branchLocked is true", () => {
    expect(
      computeBranchDisabledReason({
        branchLocked: true,
        hasRepo: true,
        branchesLoading: false,
        optionCount: 5,
      }),
    ).toContain("local executor");
  });

  it("asks to select a repo first when none is set", () => {
    expect(
      computeBranchDisabledReason({
        branchLocked: false,
        hasRepo: false,
        branchesLoading: false,
        optionCount: 0,
      }),
    ).toBe("Select a repository first.");
  });

  it("reports loading while branches are being fetched", () => {
    expect(
      computeBranchDisabledReason({
        branchLocked: false,
        hasRepo: true,
        branchesLoading: true,
        optionCount: 0,
      }),
    ).toContain("Loading");
  });

  it("reports no branches available when the list is empty", () => {
    expect(
      computeBranchDisabledReason({
        branchLocked: false,
        hasRepo: true,
        branchesLoading: false,
        optionCount: 0,
      }),
    ).toContain("No branches");
  });

  it("returns undefined when the chip should be enabled", () => {
    expect(
      computeBranchDisabledReason({
        branchLocked: false,
        hasRepo: true,
        branchesLoading: false,
        optionCount: 5,
      }),
    ).toBeUndefined();
  });
});
