import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

import { describe, expect, it } from "vitest";

const SCRIPT = path.resolve(import.meta.dirname, "i18n-sweep.mjs");

type Fixture = Record<string, string>;

interface SweepResult {
  status: number | null;
  stdout: string;
  stderr: string;
  /** Findings under "English plural concatenation" — defects. */
  plurals: string[];
  /** Findings under "literal(s) to review by eye" — judgement calls. */
  eyeReview: string[];
}

/** Split the report on its two headers so a test can assert which half a finding landed in. */
function parseSections(stdout: string): Pick<SweepResult, "plurals" | "eyeReview"> {
  const lines = stdout.split("\n");
  const eyeHeader = lines.findIndex((l) => l.includes("literal(s) to review by eye"));
  const isFinding = (l: string) => /^\s+\S+:\d+:/.test(l);
  return {
    plurals: lines.slice(0, eyeHeader).filter(isFinding),
    eyeReview: lines.slice(eyeHeader).filter(isFinding),
  };
}

function run(...args: string[]): SweepResult {
  const result = spawnSync(process.execPath, [SCRIPT, ...args], { encoding: "utf8" });
  return {
    status: result.status,
    stdout: result.stdout,
    stderr: result.stderr,
    ...parseSections(result.stdout),
  };
}

/** Materialise a fixture tree and hand its root to `use`, cleaning up afterwards. */
function withFixture<T>(fixture: Fixture, use: (root: string) => T): T {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-i18n-sweep-"));
  try {
    for (const [file, source] of Object.entries(fixture)) {
      const full = path.join(root, file);
      mkdirSync(path.dirname(full), { recursive: true });
      writeFileSync(full, source);
    }
    return use(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function sweep(fixture: Fixture): SweepResult {
  return withFixture(fixture, (root) => run(root));
}

describe("English plural concatenation", () => {
  it("reports a morpheme concatenated onto a count in JSX", () => {
    const result = sweep({
      "file-count.tsx": [
        "export function FileCount({ n }: { n: number }) {",
        '  return <span>{n} file{n === 1 ? "" : "s"}</span>;',
        "}",
      ].join("\n"),
    });

    expect(result.plurals).toHaveLength(1);
    expect(result.plurals[0]).toContain("file-count.tsx:2");
  });

  it("reports both orderings of the conditional", () => {
    const result = sweep({
      "orderings.ts": [
        'export const a = (n: number) => `${n} item${n !== 1 ? "s" : ""}`;',
        'export const b = (n: number) => `${n} item${n === 1 ? "" : "s"}`;',
      ].join("\n"),
    });

    expect(result.plurals).toHaveLength(2);
  });

  it("reports a plural morpheme hoisted into a helper variable", () => {
    const result = sweep({
      "helper.ts": [
        "export function summary(count: number) {",
        '  const plural = count === 1 ? "" : "s";',
        "  return `${count} match${plural}`;",
        "}",
      ].join("\n"),
    });

    expect(result.plurals).toHaveLength(1);
    expect(result.plurals[0]).toContain("helper.ts:2");
  });

  it("does not report a t() call using count with _one/_other keys", () => {
    const result = sweep({
      "translated.tsx": [
        'import { useTranslation } from "react-i18next";',
        "",
        "export function FileCount({ n }: { n: number }) {",
        "  const { t } = useTranslation();",
        '  return <span>{t("common:fileCount", { count: n })}</span>;',
        "}",
      ].join("\n"),
    });

    expect(result.plurals).toEqual([]);
  });

  it("does not report a ternary that selects whole words rather than a morpheme", () => {
    const result = sweep({
      "words.ts": ['export const verb = (n: number) => (n === 1 ? "is" : "are");'].join("\n"),
    });

    expect(result.plurals).toEqual([]);
  });
});

/**
 * The regression that matters. Filtering literals by length *during* the scan
 * desynchronises quote pairing: `"xs"` is skipped, so its closing quote pairs
 * with the next string's opening quote and the gap between them (`") return "`)
 * is reported as copy. That produced 25 false positives on a directory with one
 * real hit, which is exactly the noise that gets a check ignored.
 */
describe("quote pairing", () => {
  it("does not turn the gap between two literals into a finding", () => {
    const result = sweep({
      "icon-size.ts": [
        "export function iconSize(size: string) {",
        '  if (size === "xs") return "h-6 w-6 shrink-0";',
        '  return "h-8 w-8 shrink-0";',
        "}",
      ].join("\n"),
    });

    expect(result.eyeReview).toEqual([]);
    expect(result.stdout).not.toContain("return");
  });

  it("still finds prose that follows a short literal on the same line", () => {
    const result = sweep({
      "mixed.ts": [
        "export function label(size: string) {",
        '  if (size === "xs") return "[layout] Compact mode selected";',
        "}",
      ].join("\n"),
    });

    expect(result.eyeReview).toHaveLength(1);
    expect(result.eyeReview[0]).toContain("Compact mode selected");
  });
});

describe("eye-review filtering", () => {
  it("skips Tailwind class strings, catalog keys and identifiers", () => {
    const result = sweep({
      "noise.tsx": [
        'export const cls = "flex items-center gap-2";',
        'export const key = "settings:deleteExecutor";',
        'export const id = "task-archive-dialog";',
        'export const url = "https://kandev.ai/docs";',
      ].join("\n"),
    });

    expect(result.eyeReview).toEqual([]);
  });

  it("skips test files, comments and imports", () => {
    const result = sweep({
      "thing.test.tsx": 'export const copy = "Archive this board";',
      "commented.ts": [
        'import { thing } from "./Archive this board";',
        '// const copy = "Archive this board";',
        ' * "Archive this board"',
      ].join("\n"),
    });

    expect(result.eyeReview).toEqual([]);
  });

  it("finds copy in a plain .ts helper the jsx-only lint rule cannot see", () => {
    const result = sweep({
      "storage-hooks.ts": [
        "export function warn(error: unknown) {",
        '  console.warn("[storage] Could not read the disk usage report:", error);',
        "}",
      ].join("\n"),
    });

    expect(result.eyeReview).toHaveLength(1);
    expect(result.eyeReview[0]).toContain("storage-hooks.ts:2");
  });

  /**
   * Characterization, not endorsement. `NOISE_VAL`'s `[a-z0-9-]+` and
   * `[A-Z][A-Z0-9_]*` branches each match a single leading character before
   * `.*$` swallows the rest, so every value starting with a letter is dropped —
   * a `const ROWS = [{ label: "Disk usage" }]` config table stays invisible.
   * Pinned here so the limit is a known property of the tool rather than a
   * surprise, and so a future widening has to change this test on purpose.
   */
  it("does not surface prose beginning with a letter — a known limit of the filter", () => {
    const result = sweep({
      "rows.ts": [
        "export const ROWS = [",
        '  { label: "Disk usage" },',
        '  { label: "Memory in use" },',
        "];",
      ].join("\n"),
    });

    expect(result.eyeReview).toEqual([]);
  });
});

/**
 * The regression this suite exists for. The sweep used to walk directories only,
 * so a file argument hit ENOTDIR inside the walk, was swallowed, and produced the
 * boilerplate footer with no findings and exit 0 — a clean bill of health for
 * input it never opened. Briefs had instructed exactly that form
 * (`pnpm run i18n:sweep <file> <file> …`), so real batches were signed off on a
 * scan of nothing.
 */
describe("path arguments", () => {
  const DEFECT = 'export const s = (n: number) => `${n} file${n === 1 ? "" : "s"}`;';
  const LOOSE = "loose.tsx";
  const SUBDIR = "nested";
  const TREE: Fixture = { [LOOSE]: DEFECT, [`${SUBDIR}/counter.ts`]: DEFECT };

  it("reports the findings in a file named directly", () => {
    const result = withFixture(TREE, (root) => run(path.join(root, LOOSE)));

    expect(result.status).toBe(0);
    expect(result.plurals).toHaveLength(1);
    expect(result.plurals[0]).toContain(`${LOOSE}:1`);
  });

  it("reports the union of file and directory arguments", () => {
    const result = withFixture(TREE, (root) =>
      run(path.join(root, SUBDIR), path.join(root, LOOSE)),
    );

    expect(result.plurals).toHaveLength(2);
    expect(result.plurals.join("\n")).toContain("counter.ts:1");
    expect(result.plurals.join("\n")).toContain(`${LOOSE}:1`);
  });

  it("counts a file reached through two arguments once", () => {
    const result = withFixture(TREE, (root) => run(root, path.join(root, LOOSE)));

    expect(result.plurals).toHaveLength(2);
  });

  it("exits non-zero on a path that does not exist rather than reporting clean", () => {
    const result = run("does/not/exist");

    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain("does/not/exist");
    expect(result.stdout).not.toContain("0 English plural concatenation(s)");
  });

  /**
   * `0 findings` alongside `scanned 0 files` is self-evidently a non-result;
   * `0 findings` alone is indistinguishable from a pass. Printing the count is
   * the cheap structural guard against the whole failure class.
   */
  it("prints how many files were scanned, and from how many arguments", () => {
    const result = withFixture(TREE, (root) =>
      run(path.join(root, SUBDIR), path.join(root, LOOSE)),
    );

    expect(result.stdout).toContain("scanned 2 file(s) from 2 argument(s)");
  });
});

describe("report shape", () => {
  /** A file with nothing to find, so the report is only its own boilerplate. */
  const CLEAN: Fixture = { "clean.ts": "export const n = 1;" };

  it("always prints both sections and exits 0, even with nothing to report", () => {
    const result = sweep(CLEAN);

    expect(result.status).toBe(0);
    expect(result.stdout).toContain("0 English plural concatenation(s)");
    expect(result.stdout).toContain("0 literal(s) to review by eye");
  });

  it("exits 0 even when defects are found — it is a human tool, not a gate", () => {
    const result = sweep({
      "defect.ts": 'export const s = (n: number) => `${n} file${n === 1 ? "" : "s"}`;',
    });

    expect(result.status).toBe(0);
  });

  it("notes the prompt-builder exclusion in the eye-review section", () => {
    const result = sweep(CLEAN);

    expect(result.stdout).toContain("agent-facing");
  });

  /**
   * The known-good prompt builders in `components/github` surface under *plural
   * concatenation*, not eye-review — an agent-facing string carrying `{n}
   * check{s}` is not a defect, because the text goes verbatim to a model. A
   * caveat printed only by the eye-review section would leave the defect list
   * telling a maintainer to break working prompts.
   */
  it("repeats the prompt-builder exclusion in the plural section when it has findings", () => {
    const result = sweep({
      "prompt-builder.ts": [
        "// Agent-facing: sent verbatim to the model, deliberately not translated.",
        "export const heading = (failed: string[]) =>",
        '  `### ${failed.length} CI Check${failed.length !== 1 ? "s" : ""} Failed`;',
      ].join("\n"),
    });

    const pluralSection = result.stdout.slice(0, result.stdout.indexOf("to review by eye"));
    expect(result.plurals).toHaveLength(1);
    expect(pluralSection).toContain("agent-facing");
    expect(pluralSection).toContain("pr-checks-section.tsx");
  });

  it("keeps the plural section bare when it has no findings", () => {
    const result = sweep(CLEAN);

    const pluralSection = result.stdout.slice(0, result.stdout.indexOf("to review by eye"));
    expect(pluralSection).not.toContain("agent-facing");
  });

  it("exits non-zero when given no path", () => {
    const result = run();

    expect(result.status).toBe(2);
    expect(result.stderr).toContain("Usage:");
  });

  it("notes that copy from a plain helper is invisible to it", () => {
    const result = sweep(CLEAN);

    expect(result.stdout).toContain("plain helper");
  });
});
