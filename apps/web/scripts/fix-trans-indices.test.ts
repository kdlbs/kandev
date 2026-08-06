import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

import { describe, expect, it } from "vitest";

const SCRIPT = path.resolve(import.meta.dirname, "fix-trans-indices.mjs");

type Fixture = Record<string, string>;

interface Run {
  status: number | null;
  stdout: string;
  stderr: string;
  /** The trailing JSON summary, or null when the script exited before printing one. */
  report: Record<string, number | boolean> | null;
}

/**
 * A `<Trans>` whose message carries two tags but whose JSX has one element child.
 * The script cannot know which phrase belongs to the element, so it declines and
 * reports — a deterministic finding to assert on.
 */
function declining(key: string): string {
  return [
    'import { Trans } from "react-i18next";',
    "",
    "export const Line = () => (",
    `  <Trans i18nKey="common:${key}">`,
    "    Hi <b>there</b>",
    "  </Trans>",
    ");",
  ].join("\n");
}

const CATALOG = JSON.stringify({
  greetingA: "Hi <0>there</0> and <1>you</1>",
  greetingB: "Hi <0>there</0> and <1>you</1>",
});

const LOOSE = "loose.tsx";
const SUBDIR = "nested";

const TREE: Fixture = {
  "src/locales/en/common.json": CATALOG,
  [LOOSE]: declining("greetingA"),
  [`${SUBDIR}/other.tsx`]: declining("greetingB"),
};

function withFixture<T>(fixture: Fixture, use: (root: string) => T): T {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-trans-indices-"));
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

/** Run against `root`'s fixture catalog so the real `src/locales/en` is never read. */
function run(root: string | null, ...args: string[]): Run {
  const env = { ...process.env };
  if (root) env.KANDEV_I18N_EN_DIR = path.join(root, "src", "locales", "en");
  const result = spawnSync(process.execPath, [SCRIPT, ...args], { encoding: "utf8", env });
  const start = result.stdout.indexOf("{");
  return {
    status: result.status,
    stdout: result.stdout,
    stderr: result.stderr,
    report: start === -1 ? null : JSON.parse(result.stdout.slice(start)),
  };
}

/**
 * The regression this suite exists for. The script walked directories only, so a
 * file argument threw ENOTDIR mid-walk and a non-existent one was skipped
 * silently — both ending at a `{"fixed": 0}` summary that reads as a verdict on
 * code the script never opened.
 */
describe("path arguments", () => {
  it("reports the findings in a file named directly", () => {
    const result = withFixture(TREE, (root) => run(root, path.join(root, LOOSE)));

    expect(result.status).toBe(0);
    expect(result.report?.scanned).toBe(1);
    expect(result.report?.declinedCount).toBe(1);
    expect(result.stdout).toContain(LOOSE);
    expect(result.stdout).toContain("common:greetingA");
  });

  it("reports the union of file and directory arguments", () => {
    const result = withFixture(TREE, (root) =>
      run(root, path.join(root, SUBDIR), path.join(root, LOOSE)),
    );

    expect(result.report?.scanned).toBe(2);
    expect(result.report?.declinedCount).toBe(2);
    expect(result.stdout).toContain("common:greetingA");
    expect(result.stdout).toContain("common:greetingB");
  });

  it("counts a file reached through two arguments once", () => {
    const result = withFixture(TREE, (root) => run(root, root, path.join(root, LOOSE)));

    expect(result.report?.scanned).toBe(2);
    expect(result.report?.declinedCount).toBe(2);
  });

  it("exits non-zero on a path that does not exist rather than reporting clean", () => {
    const result = run(null, "does/not/exist");

    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain("does/not/exist");
    expect(result.report).toBeNull();
  });

  it("prints how many files were scanned, and from how many arguments", () => {
    const result = withFixture(TREE, (root) =>
      run(root, path.join(root, SUBDIR), path.join(root, LOOSE)),
    );

    expect(result.stdout).toContain("scanned 2 file(s) from 2 argument(s)");
  });

  it("leaves the catalog alone without --write", () => {
    const result = withFixture(TREE, (root) => run(root, path.join(root, LOOSE)));

    expect(result.report?.wrote).toBe(false);
  });
});
