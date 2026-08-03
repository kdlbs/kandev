import { describe, expect, it } from "vitest";
import { accountedFor, buildCatalog, literals, looksLikeCopy } from "./removed-literals.mjs";
import { isCandidate } from "./changed-files.mjs";

/**
 * The failure this check exists for, reproduced exactly.
 *
 * A live route rendered one sentence; a dead route migrated earlier had left a
 * plausible-looking key holding a DIFFERENT sentence. Pointing the live route at
 * that key silently rewrote user-visible English, and lint, all four
 * `i18n:check` gates, the ratchet and 8,144 unit tests were clean — every string
 * was externalized, the key existed, the catalogs were in sync.
 */
const LIVE_COPY = "Create a diagnostic ZIP with frontend and backend logs.";
const REUSED_KEY_COPY = "Download a bounded diagnostic ZIP containing frontend and backend logs.";

describe("accountedFor", () => {
  it("reports a removed sentence whose replacement key holds different English", () => {
    const catalog = buildCatalog([REUSED_KEY_COPY]);

    expect(accountedFor(LIVE_COPY, catalog)).toBe(false);
  });

  it("accepts a string that became a catalog message unchanged", () => {
    expect(accountedFor(LIVE_COPY, buildCatalog([LIVE_COPY]))).toBe(true);
  });

  it("accepts a value extracted into an interpolation", () => {
    const catalog = buildCatalog(["Files over {{limit}} are skipped when copying to remote."]);

    expect(accountedFor("Files over 5 MiB are skipped when copying to remote.", catalog)).toBe(
      true,
    );
  });

  it("accepts either half of a <Trans> sentence split around an element", () => {
    const catalog = buildCatalog([
      "Type the workspace name <0>{{name}}</0> to confirm deletion. This action cannot be undone.",
    ]);

    expect(accountedFor("Type the workspace name", catalog)).toBe(true);
    expect(accountedFor("to confirm deletion. This action cannot be undone.", catalog)).toBe(true);
  });

  it("does not accept a rewrite merely because it shares words with the message", () => {
    const catalog = buildCatalog(["Delete the repository and every script attached to it."]);

    expect(accountedFor("Delete the repository and its scripts.", catalog)).toBe(false);
  });

  it("accepts a plural pair rendered from the singular source sentence", () => {
    const catalog = buildCatalog(["{{count}} custom script", "{{count}} custom scripts"]);

    expect(accountedFor("2 custom scripts", catalog)).toBe(true);
  });
});

describe("literals", () => {
  it("sees JSX text, which carries no quotes and is most migrated copy", () => {
    const found: Set<string> | null = literals(
      `export const A = () => <Link>Back to Workspaces</Link>;`,
      "a.tsx",
    );

    expect(found?.has("Back to Workspaces")).toBe(true);
  });

  it("sees string literals in attributes and template quasis", () => {
    const found: Set<string> | null = literals(
      'export const A = () => <input placeholder="Filter repositories..." aria-label={`Select ${x}`} />;',
      "a.tsx",
    );

    expect(found?.has("Filter repositories...")).toBe(true);
    expect(found?.has("Select")).toBe(true);
  });

  it("returns null for an unparseable file rather than reporting it as empty", () => {
    expect(literals("export const = ??", "a.tsx")).toBeNull();
  });
});

describe("looksLikeCopy", () => {
  it.each([
    ["Delete Repository", true],
    ["Manage your workspaces and workflows", true],
    ["/absolute/path/to/repository", false],
    ["https://example.com/docs", false],
    ["worktree_branch_template", false],
    ["repositoryName", false],
    ["move_to_next", false],
    ["px-1 py-0.5", false],
    // A catalog reference. Re-pointing a t() call reports the OLD key as a
    // removed literal, because the key is itself a StringLiteral.
    ["workspaces:noScriptsYet", false],
    ["common:requestFailed", false],
    // Guard: prose containing a colon is still copy.
    ["Supported patterns:", true],
    ["Note: this cannot be undone", true],
  ])("classifies %j as copy=%s", (value, expected) => {
    expect(looksLikeCopy(value)).toBe(expected);
  });
});

/**
 * Two false positives were reported from a large migration and traced to an
 * earlier regex PROTOTYPE of this check rather than to this module. Pinned here
 * because the reports were circulated as bugs, and the obvious "fix" for either
 * would make the check worse: scanning catalogs would report keys as copy, and
 * widening `{{…}}` further would weaken the rewrite detection that is the whole
 * point.
 */
describe("reported false positives that this module does not have", () => {
  it("matches a literal whose leading number moved into an interpolation", () => {
    // Reported as "permanently unmatchable". Rule 2 turns {{…}} into a wildcard,
    // so a placeholder at the START of the message matches just as well as an
    // interior one — provided the message has two words to anchor on.
    expect(accountedFor("3 of 9 selected", buildCatalog(["{{count}} of {{total}} selected"]))).toBe(
      true,
    );
    expect(accountedFor("50 items remaining", buildCatalog(["{{count}} items remaining"]))).toBe(
      true,
    );
  });

  it("still reports a rewrite that only differs where a placeholder is not", () => {
    // The guard on the rule above: a wildcard must not swallow changed prose.
    expect(accountedFor("50 max", buildCatalog(["{{count}} maximum"]))).toBe(false);
  });

  it("never treats a catalog as a source of candidate literals", () => {
    // Reported as "a whole-file JSON reformat floods the output with catalog
    // KEYS". Catalogs are what the check compares *against*; `isCandidate`
    // admits only .ts/.tsx, so reformatting one cannot produce a single
    // candidate. Scanning them would report keys as removed copy.
    expect(isCandidate("apps/web/src/locales/en/common.json")).toBe(false);
    expect(isCandidate("apps/web/src/locales/pseudo/workspaces.json")).toBe(false);
    expect(isCandidate("apps/web/components/settings/repository-card.tsx")).toBe(true);
  });
});

/**
 * The assertions the rest of this file structurally cannot make: they all assume
 * a well-formed catalog, so none of them can tell a working check from one that
 * accepts everything. A check that silently passes is worse than no check, so
 * these prove it can still FAIL.
 */
describe("a degenerate catalog message cannot disable the check", () => {
  it("does not let a message that is only a placeholder match everything", () => {
    // `settings:externalMcpConfigPath` was literally "{{path}}", which compiled
    // to /^[\s\S]+?$/ and made accountedFor return true for every string in the
    // repo — the whole check became a no-op that still printed a tick.
    expect(accountedFor("anything at all", buildCatalog(["{{x}}"]))).toBe(false);
    expect(accountedFor("Delete a workflow and all its steps.", buildCatalog(["{{path}}"]))).toBe(
      false,
    );
  });

  it("does not let a message with no word in it anchor a pattern", () => {
    // "{{a}} —" normalizes to "—" and would compile to /^[\s\S]+? —$/.
    expect(accountedFor("totally unrelated prose —", buildCatalog(["{{a}} —"]))).toBe(false);
  });

  it("still matches such a message exactly and as a fragment", () => {
    // Dropping its PATTERN must not drop the message from the catalog.
    expect(accountedFor("{{path}}", buildCatalog(["{{path}}"]))).toBe(true);
  });

  it("reports a value extraction whose message is too thin to anchor", () => {
    // "{{count}} of {{total}}" reduces to the residue "of" — BYTE-IDENTICAL to
    // "{{a}} of {{b}}", which would match "Anything at all of anything else".
    // Nothing computed from the message can accept one and reject the other, so
    // both lose their pattern and "3 of 9" is reported for a human to classify.
    // A false positive costs ten seconds; a false negative is the failure this
    // check exists to prevent.
    expect(accountedFor("3 of 9", buildCatalog(["{{count}} of {{total}}"]))).toBe(false);
  });

  it("does not let one short word anchor a nearly unbounded pattern", () => {
    // "Delete {{name}}" compiled to /^Delete [\s\S]+?$/ and accepted every
    // "Delete …" sentence — 19 of 43 strings in the file this check was first
    // pointed at were unprotected by exactly this shape.
    expect(
      accountedFor("Delete a workflow and all its steps.", buildCatalog(["Delete {{name}}"])),
    ).toBe(false);
    expect(accountedFor("Confirm", buildCatalog(["{{count}}m"]))).toBe(false);
  });

  it("keeps anchoring once a message has two words of prose", () => {
    expect(
      accountedFor(
        "Files over 5 MiB are skipped",
        buildCatalog(["Files over {{limit}} are skipped"]),
      ),
    ).toBe(true);
    expect(
      accountedFor("totally unrelated prose", buildCatalog(["Files over {{limit}} are skipped"])),
    ).toBe(false);
  });
});
