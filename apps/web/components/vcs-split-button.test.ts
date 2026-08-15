import { describe, expect, it, vi } from "vitest";

import { t } from "@/lib/i18n";
import {
  buildVcsSplitCallbacks,
  determinePrimaryAction,
  hasOpenChangeRequest,
} from "./vcs-split-button";
import { buildSingleRepoContributionCallbacks } from "./vcs-split-button-parts";

/**
 * The VCS tooltips used to build their plural by hand:
 *
 *   `Create PR (${aheadCount} commit${aheadCount !== 1 ? "s" : ""} ahead)`
 *
 * That puts the plural rule at the call site where no other locale can reach
 * it, so the copy moved to `count` + `_one`/`_other`. These assertions pin the
 * English to exactly what the concatenation emitted — E2E specs and the
 * accessible-name queries depend on it, and a plural key is the easiest place
 * to silently reword a string while every gate stays green.
 */
describe("vcs split-button count-bearing copy", () => {
  const legacyCommit = (n: number) => `Commit ${n} changed file${n !== 1 ? "s" : ""}`;
  const legacyPr = (n: number) => `Create PR (${n} commit${n !== 1 ? "s" : ""} ahead)`;
  const legacyPush = (n: number) => `Push ${n} commit${n !== 1 ? "s" : ""} to remote`;

  // 1 is the only value the old ternary treated specially; 0 and 2 both take
  // the "s" branch, and 0 is the one a naive `n > 1` port would get wrong.
  for (const n of [0, 1, 2, 5]) {
    it(`matches the pre-migration English for ${n} commit(s)`, () => {
      expect(t("integrations:commitChangedFiles", { count: n })).toBe(legacyCommit(n));
      expect(t("integrations:createPrCommitsAhead", { count: n })).toBe(legacyPr(n));
      expect(t("integrations:pushCommitsToRemote", { count: n })).toBe(legacyPush(n));
    });
  }

  it("interpolates the git ref rather than translating it", () => {
    expect(t("integrations:ontoBranch", { branch: "origin/main" })).toBe("onto origin/main");
    expect(t("integrations:fromBranch", { branch: "release/1.2" })).toBe("from release/1.2");
    expect(t("integrations:rebaseOntoBranchBehind", { branch: "origin/main", behind: 3 })).toBe(
      "Rebase onto origin/main (3 behind)",
    );
  });

  it("keeps the divergence aria-labels count-invariant, as the shipped English was", () => {
    expect(t("integrations:commitsAheadAriaLabel", { value: 1 })).toBe("1 commits ahead");
    expect(t("integrations:commitsBehindAriaLabel", { value: 2 })).toBe("2 commits behind");
  });
});

describe("hasOpenChangeRequest", () => {
  it("recognizes a registered provider review as the native task change request", () => {
    expect(
      hasOpenChangeRequest(undefined, [
        {
          providerId: "bitbucket",
          reviewKey: "workspace/repo#42",
          title: "Fix auth",
          url: "https://bitbucket.test/pull-requests/42",
          connectionScope: "https://bitbucket.test",
          repositoryId: "repository-1",
          changeRequestNumber: 42,
          state: "OPEN",
        },
      ]),
    ).toBe(true);
  });

  it("does not treat merged or closed reviews as open", () => {
    expect(
      hasOpenChangeRequest("closed", [
        {
          providerId: "bitbucket",
          reviewKey: "workspace/repo#42",
          title: "Fix auth",
          url: "https://bitbucket.test/pull-requests/42",
          connectionScope: "https://bitbucket.test",
          repositoryId: "repository-1",
          changeRequestNumber: 42,
          state: "MERGED",
        },
      ]),
    ).toBe(false);
  });
});

describe("vcs split-button remote action semantics", () => {
  it("uses the upstream-relative count for a linked PR push", () => {
    expect(determinePrimaryAction(0, 1, 7, true)).toBe("push");
  });

  it("does not select Push when divergence policy removes unsafe push evidence", () => {
    expect(determinePrimaryAction(0, 0, 7, true)).toBe("rebase");
  });

  it("passes the blocked repository to single-repository contribution actions", async () => {
    const requestReplace = vi.fn();
    const requestUse = vi.fn();
    const openExternalLinkFn = vi.fn().mockResolvedValue("browser");
    const resolutionTarget = {
      expectedRemoteHead: "a".repeat(40),
      repo: "widget-a",
      repositoryName: "widget-a",
    };
    const callbacks = buildVcsSplitCallbacks({
      openCommitDialog: vi.fn(),
      openPRDialog: vi.fn(),
      handlePull: vi.fn(),
      handlePush: vi.fn(),
      handleRebase: vi.fn(),
      handleMerge: vi.fn(),
      resolution: { requestReplace, requestUse },
      resolutionTarget,
      selectedPR: { pr_url: "https://github.com/acme/widget-a/pull/42" },
      openExternalLinkFn,
    });

    callbacks.onReplaceContribution("widget-a");
    callbacks.onUseContribution("widget-a");
    callbacks.onViewContribution("widget-a");
    await Promise.resolve();

    expect(requestReplace).toHaveBeenCalledWith(resolutionTarget);
    expect(requestUse).toHaveBeenCalledWith(resolutionTarget);
    expect(openExternalLinkFn).toHaveBeenCalledWith("https://github.com/acme/widget-a/pull/42");
  });

  it("scopes the single-repository menu callbacks to the blocked repository", () => {
    const onReplaceContribution = vi.fn();
    const onUseContribution = vi.fn();
    const onViewContribution = vi.fn();
    const callbacks = buildSingleRepoContributionCallbacks(
      { onReplaceContribution, onUseContribution, onViewContribution },
      "widget-a",
    );

    callbacks.onReplaceContribution();
    callbacks.onUseContribution();
    callbacks.onViewContribution();

    expect(onReplaceContribution).toHaveBeenCalledWith("widget-a");
    expect(onUseContribution).toHaveBeenCalledWith("widget-a");
    expect(onViewContribution).toHaveBeenCalledWith("widget-a");
  });
});

/**
 * The discard dialog used to concatenate a " in your local clone at X" tail
 * onto a shared stem, which froze the tail's position in the sentence. Each
 * branch is now a whole message; these pin both to the previous rendering.
 */
describe("discard-local-changes description", () => {
  it("reads identically to the old stem + tail concatenation", () => {
    const stem = "Starting this task will permanently discard the uncommitted changes";
    const tail = "Back up anything you want to keep before continuing.";

    expect(t("common:discardLocalChangesDescription")).toBe(`${stem} in your local clone. ${tail}`);
    expect(t("common:discardLocalChangesAtPathDescription", { repoPath: "/srv/repo" })).toBe(
      `${stem} in your local clone at /srv/repo. ${tail}`,
    );
  });
});
