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

function sweep(fixture: Fixture): SweepResult {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-i18n-sweep-"));
  try {
    for (const [file, source] of Object.entries(fixture)) {
      const full = path.join(root, file);
      mkdirSync(path.dirname(full), { recursive: true });
      writeFileSync(full, source);
    }
    const result = spawnSync(process.execPath, [SCRIPT, root], { encoding: "utf8" });
    return { status: result.status, stdout: result.stdout, ...parseSections(result.stdout) };
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
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

describe("report shape", () => {
  it("always prints both sections and exits 0, even with nothing to report", () => {
    const result = sweep({ "clean.ts": "export const n = 1;" });

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
    const result = sweep({ "clean.ts": "export const n = 1;" });

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
    const result = sweep({ "clean.ts": "export const n = 1;" });

    const pluralSection = result.stdout.slice(0, result.stdout.indexOf("to review by eye"));
    expect(pluralSection).not.toContain("agent-facing");
  });

  it("exits non-zero when given no directory", () => {
    const result = spawnSync(process.execPath, [SCRIPT], { encoding: "utf8" });

    expect(result.status).toBe(2);
    expect(result.stderr).toContain("Usage:");
  });
});
