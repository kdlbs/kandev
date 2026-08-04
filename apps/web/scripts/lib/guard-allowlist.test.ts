/**
 * Duplicate detection for `i18nGuardFiles`, and the invariant it protects.
 *
 * The bug this pins down (#2214): the sidebar migration appended two paths that
 * earlier migrations had already listed. A duplicate changes no behaviour — the
 * ESLint config just passes the same pattern twice — so it cleared `pnpm lint`,
 * `i18n:check`, the ratchet, `check-guard-allowlist.mjs` itself and 8,218 unit
 * tests. It was caught by eye. What it costs is the record: the array is read as
 * "which PR migrated what", and a second copy silently books someone else's work
 * as the new PR's coverage.
 *
 * Following the pattern in `git-base.test.ts`, the "fixed" assertions are paired
 * with one showing the pre-existing removal check still cannot see a duplicate —
 * a test that only exercised the new helper would not explain why it has to be a
 * separate check.
 */
import { describe, expect, it } from "vitest";

import { i18nGuardFiles } from "../../eslint.i18n.options.mjs";
import { duplicateEntries } from "./guard-allowlist.mjs";

/** What `check-guard-allowlist.mjs` did before this helper existed. */
function removedEntries(before: string[], after: string[]) {
  const afterSet = new Set(after);
  return before.filter((entry) => !afterSet.has(entry));
}

describe("duplicateEntries", () => {
  it("reports nothing for a list with no repeats", () => {
    expect(duplicateEntries(["a.tsx", "b.tsx", "components/c/**"])).toEqual([]);
  });

  it("names an entry listed twice", () => {
    const entries = ["a.tsx", "b.tsx", "a.tsx"];

    expect(duplicateEntries(entries)).toEqual(["a.tsx"]);
  });

  it("names an entry once however often it repeats", () => {
    expect(duplicateEntries(["a.tsx", "a.tsx", "a.tsx", "a.tsx"])).toEqual(["a.tsx"]);
  });

  it("names every distinct repeat — #2214 added two, not one", () => {
    const entries = [
      "components/app-sidebar/sections/settings/general-group.tsx",
      "components/app-sidebar/sections/settings/agents-group.tsx",
      "components/app-status-bar/**/*.{ts,tsx}",
      "components/app-sidebar/sections/settings/general-group.tsx",
      "components/app-sidebar/sections/settings/agents-group.tsx",
    ];

    expect(duplicateEntries(entries)).toEqual([
      "components/app-sidebar/sections/settings/general-group.tsx",
      "components/app-sidebar/sections/settings/agents-group.tsx",
    ]);
  });

  /**
   * The check is exact-match ON PURPOSE. `main` carries entries that a broader
   * glob already covers, and #2202's comment keeps them deliberately: they are
   * the historical record of which PR migrated which half of the System group.
   * Widening this into a subsumption check would fail the list as it stands.
   */
  it("does not flag an entry a broader glob already covers", () => {
    const entries = [
      "app/settings/system/**/*.{ts,tsx}",
      "app/settings/system/storage/**/*.{ts,tsx}",
      "components/settings/system/*.{ts,tsx}",
      "components/settings/system/system-page-shell.tsx",
    ];

    expect(duplicateEntries(entries)).toEqual([]);
  });
});

describe("the removal check cannot stand in for it", () => {
  it("reproduces the gap: adding a duplicate removes nothing", () => {
    const before = ["a.tsx", "b.tsx"];
    const after = ["a.tsx", "b.tsx", "a.tsx"];

    expect(removedEntries(before, after)).toEqual([]);
    expect(duplicateEntries(after)).toEqual(["a.tsx"]);
  });
});

describe("the live allowlist", () => {
  it("lists no path twice", () => {
    expect(duplicateEntries(i18nGuardFiles)).toEqual([]);
  });

  /**
   * Guards the scenario above against drift: if these two ever stop coexisting,
   * the "exact duplicates only" test is no longer pinning a real decision.
   */
  it("still carries the deliberate storage redundancy the check must tolerate", () => {
    expect(i18nGuardFiles).toContain("app/settings/system/**/*.{ts,tsx}");
    expect(i18nGuardFiles).toContain("app/settings/system/storage/**/*.{ts,tsx}");
  });
});
