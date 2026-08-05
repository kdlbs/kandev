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
import * as nodeFs from "node:fs";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { i18nGuardFiles } from "../../eslint.i18n.options.mjs";
import { duplicateEntries, filesForEntry, unmatchedEntries } from "./guard-allowlist.mjs";

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

/**
 * The born-dead entry (#2247). A glob for the `[id]` automations route was
 * appended by a migration and matched NOTHING: glob brackets are a CHARACTER CLASS, so `[id]`
 * means "one `i` or one `d`" and never the literal directory. The route was
 * allowlisted and unguarded at the same time, and every check reported green —
 * it never left the array so the removal check ignored it, it was not repeated
 * so `duplicateEntries` ignored it, and `pnpm lint` passed because the rule had
 * no files to apply. Confirmed then by putting a hardcoded literal in that
 * route: 0 lint errors before escaping, 1 after.
 *
 * These run the REAL `fs.globSync` against a fixture tree rather than a
 * re-implementation of glob semantics — the bracket behaviour is the whole point,
 * so emulating it would only test the emulation. **Do not simplify this back to a
 * hand-rolled matcher.** The first draft did exactly that, and a malformed
 * character class in it (`/[*?[]]{}]/` — a lost backslash) made EVERY entry look
 * unmatched, so the live-allowlist test below reported a problem that did not
 * exist. A false positive that reads as a find is the failure mode this whole
 * area keeps producing; a fixture plus the real matcher cannot have it.
 *
 * As with `duplicateEntries` above, the helper is paired with a test showing both
 * existing checks are blind to it, so the reason it must be separate stays on the
 * record rather than becoming folklore.
 */
describe("unmatchedEntries", () => {
  let root: string;

  /**
   * The very resolver `check-guard-allowlist.mjs` uses, pointed at the fixture —
   * not a re-implementation. Glob bracket semantics are the thing under test, so
   * emulating them would only test the emulation.
   */
  const resolve = (entry: string) => filesForEntry(entry, { cwd: root, fsImpl: nodeFs });

  beforeAll(() => {
    root = mkdtempSync(path.join(tmpdir(), "kandev-guard-allowlist-"));
    for (const file of [
      "app/settings/workspace/[id]/automations/page.tsx",
      "app/settings/workspace/[id]/automations/new/page.tsx",
      "components/automations/trigger-card.tsx",
    ]) {
      const full = path.join(root, file);
      mkdirSync(path.dirname(full), { recursive: true });
      writeFileSync(full, "");
    }
  });

  afterAll(() => rmSync(root, { recursive: true, force: true }));

  it("reports nothing when every entry matches", () => {
    expect(unmatchedEntries(["components/automations/trigger-card.tsx"], resolve)).toEqual([]);
    expect(unmatchedEntries(["components/automations/*.tsx"], resolve)).toEqual([]);
  });

  it("names an unescaped dynamic-route entry, which matches nothing", () => {
    const entry = "app/settings/workspace/[id]/automations/**/*.tsx";

    expect(resolve(entry)).toEqual([]);
    expect(unmatchedEntries([entry], resolve)).toEqual([entry]);
  });

  it("accepts the escaped form of the very same route", () => {
    const entry = "app/settings/workspace/[[]id[]]/automations/**/*.tsx";

    expect(resolve(entry)).toHaveLength(2);
    expect(unmatchedEntries([entry], resolve)).toEqual([]);
  });

  it("names an entry whose path is gone", () => {
    expect(unmatchedEntries(["components/gone/page.tsx"], resolve)).toEqual([
      "components/gone/page.tsx",
    ]);
  });

  it("names every dead entry, not only the first", () => {
    const dead = ["components/gone/a.tsx", "components/gone/b.tsx"];
    const entries = [...dead, "components/automations/trigger-card.tsx"];

    expect(unmatchedEntries(entries, resolve)).toEqual(dead);
  });
});

describe("neither existing check can stand in for it", () => {
  it("reproduces the gap: a born-dead entry never leaves the array and is not a duplicate", () => {
    const before = ["a.tsx"];
    const after = ["a.tsx", "app/settings/workspace/[id]/automations/**/*.tsx"];

    // Both existing checks are clean: nothing left, nothing repeated.
    expect(removedEntries(before, after)).toEqual([]);
    expect(duplicateEntries(after)).toEqual([]);
    // Only the new one sees it.
    expect(unmatchedEntries(after, () => [])).toEqual(after);
  });
});

describe("the live allowlist", () => {
  /**
   * If this fails, some entry is listed but guards nothing. Run
   * `node scripts/check-guard-allowlist.mjs` — it names the entry and the fix.
   */
  it("has no entry matching zero files", () => {
    const webDir = path.resolve(import.meta.dirname, "..", "..");
    const live = (entry: string) => filesForEntry(entry, { cwd: webDir, fsImpl: nodeFs });

    expect(unmatchedEntries(i18nGuardFiles, live)).toEqual([]);
  });
});
