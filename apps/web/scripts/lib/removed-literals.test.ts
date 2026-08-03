import { describe, expect, it } from "vitest";
import { accountedFor, buildCatalog, literals, looksLikeCopy } from "./removed-literals.mjs";

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
  ])("classifies %j as copy=%s", (value, expected) => {
    expect(looksLikeCopy(value)).toBe(expected);
  });
});
