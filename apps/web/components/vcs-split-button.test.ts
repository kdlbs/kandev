import { describe, expect, it } from "vitest";

import { t } from "@/lib/i18n";

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
