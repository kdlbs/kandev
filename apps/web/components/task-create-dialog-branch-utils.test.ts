import { afterEach, describe, it, expect } from "vitest";
import { i18n } from "@/lib/i18n";
import {
  computeBranchIntent,
  computeBranchPrefix,
  computeBranchTooltip,
  computeBranchDisabledReason,
} from "./task-create-dialog-branch-utils";

afterEach(async () => {
  await i18n.changeLanguage("en");
});

describe("computeBranchIntent", () => {
  it('returns "none" when no branch is picked yet', () => {
    expect(
      computeBranchIntent({
        isLocalExecutor: true,
        rowBranch: "",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("none");
  });

  it('returns "current" for local executor when row branch matches workspace current', () => {
    expect(
      computeBranchIntent({
        isLocalExecutor: true,
        rowBranch: "main",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("current");
  });

  it('returns "current" on detached HEAD when the picked value is the same short SHA', () => {
    // Backend returns a short SHA in `current_branch` when the repo is on
    // detached HEAD; the autoselect seeds row.branch with that same value
    // so the chip can read "current: <sha>" instead of falling through to
    // "will switch to: master".
    expect(
      computeBranchIntent({
        isLocalExecutor: true,
        rowBranch: "4fbc5d7",
        currentLocalBranch: "4fbc5d7",
        freshBranchEnabled: false,
      }),
    ).toBe("current");
  });

  it('returns "switch" for local executor when row branch differs from current', () => {
    expect(
      computeBranchIntent({
        isLocalExecutor: true,
        rowBranch: "develop",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("switch");
  });

  it("treats unknown current branch as 'will switch to' on local executor", () => {
    // Local-status fetch error leaves currentLocalBranch="". With no baseline
    // we can't claim the picked branch is "current", so fall through to the
    // destructive label.
    expect(
      computeBranchIntent({
        isLocalExecutor: true,
        rowBranch: "main",
        currentLocalBranch: "",
        freshBranchEnabled: false,
      }),
    ).toBe("switch");
  });

  it('returns "from" for non-local executors regardless of current branch', () => {
    expect(
      computeBranchIntent({
        isLocalExecutor: false,
        rowBranch: "main",
        currentLocalBranch: "main",
        freshBranchEnabled: false,
      }),
    ).toBe("from");
    expect(
      computeBranchIntent({
        isLocalExecutor: false,
        rowBranch: "develop",
        currentLocalBranch: "",
        freshBranchEnabled: false,
      }),
    ).toBe("from");
  });

  it('returns "from" when fork-a-new-branch is enabled, even on local executor', () => {
    expect(
      computeBranchIntent({
        isLocalExecutor: true,
        rowBranch: "main",
        currentLocalBranch: "main",
        freshBranchEnabled: true,
      }),
    ).toBe("from");
    expect(
      computeBranchIntent({
        isLocalExecutor: true,
        rowBranch: "develop",
        currentLocalBranch: "main",
        freshBranchEnabled: true,
      }),
    ).toBe("from");
  });
});

describe("computeBranchPrefix", () => {
  it("renders the qualifier for each intent and nothing for none", () => {
    expect(computeBranchPrefix("none")).toBe("");
    expect(computeBranchPrefix("current")).toBe("current: ");
    expect(computeBranchPrefix("switch")).toBe("will switch to: ");
    expect(computeBranchPrefix("from")).toBe("from: ");
  });

  it("resolves at call time, so a locale switch reaches the qualifier", async () => {
    await i18n.changeLanguage("pseudo");
    expect(computeBranchPrefix("switch")).toMatch(/[^\p{ASCII}]/u);
  });
});

describe("computeBranchTooltip", () => {
  it('returns the "current" tooltip for the current intent', () => {
    expect(computeBranchTooltip("current")).toContain("as-is");
  });

  it('returns the "switch" tooltip for the switch intent', () => {
    expect(computeBranchTooltip("switch")).toContain("git checkout");
  });

  it('returns the "fork" tooltip for the from intent', () => {
    expect(computeBranchTooltip("from")).toContain("forked");
  });

  it("returns a generic tooltip for none or undefined", () => {
    expect(computeBranchTooltip("none")).toContain("Branch the agent will run against");
    expect(computeBranchTooltip(undefined)).toContain("Branch the agent will run against");
  });

  it("keeps prefix and tooltip paired under a non-English locale", async () => {
    // The regression this guards: the tooltip used to be selected by matching
    // the prefix STRING with `===`. Translating the prefix would have made every
    // branch miss and collapsed all three tooltips to the generic one.
    await i18n.changeLanguage("pseudo");
    const specific = new Set(
      (["current", "switch", "from"] as const).map((intent) => computeBranchTooltip(intent)),
    );
    expect(specific.size).toBe(3);
    expect(specific.has(computeBranchTooltip("none"))).toBe(false);
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
