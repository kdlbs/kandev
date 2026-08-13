/**
 * The sleep ratchet driven end to end against real git topologies.
 *
 * The rule has its own RuleTester suite and the diff parser has its own; those
 * prove the parts. This proves they are *connected* — that a sleep on a new line
 * actually reaches an exit code, and that a sleep on an untouched line actually
 * does not. A gate whose matcher is perfect but whose wiring is broken reports a
 * clean tree forever, which is the one failure mode nobody goes looking for.
 *
 * Every scenario is a real repository with the real `apps/web/e2e/…` layout and
 * the real eslint config, so nothing here can pass by mocking the thing under
 * test.
 */
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterAll, describe, expect, it } from "vitest";

import { findNewSleeps } from "./e2e-sleep-ratchet.mjs";

const REAL_WEB_DIR = path.resolve(import.meta.dirname, "../..");
const CONFIG_FILE = path.join(REAL_WEB_DIR, "eslint.e2e-sleeps.config.mjs");

const SPEC = "apps/web/e2e/tests/demo/demo.spec.ts";

/** The line each scenario inserts before, extracted because it appears in five. */
const ANCHOR = '  await page.click("#done");';

/** A spec with a sleep at a known line, plus filler to edit elsewhere. */
const LEGACY_SPEC = [
  'import { test } from "@playwright/test";', // 1
  "", // 2
  'test("legacy", async ({ page }) => {', // 3
  '  await page.goto("/");', // 4
  "  await page.waitForTimeout(500);", // 5  <- the pre-existing sleep
  ANCHOR, // 6
  "});", // 7
  "", // 8
].join("\n");

const tmpRoots: string[] = [];
afterAll(() => {
  for (const dir of tmpRoots) fs.rmSync(dir, { recursive: true, force: true });
});

function git(cwd: string, args: string[]) {
  return execFileSync(
    "git",
    [
      "-c",
      "user.email=ratchet@example.test",
      "-c",
      "user.name=Ratchet Test",
      "-c",
      "commit.gpgsign=false",
      // A developer machine may set core.hooksPath globally; an inherited
      // pre-commit hook would fail these fixture commits for unrelated reasons.
      "-c",
      "core.hooksPath=/nonexistent",
      ...args,
    ],
    { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
  );
}

function write(dir: string, rel: string, body: string) {
  const file = path.join(dir, rel);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, body);
}

/**
 * Stage everything, which is what makes a NEW file visible to the ratchet.
 *
 * `git diff <base>` compares a commit against the working tree and does not
 * report untracked paths at all, so an unstaged new file is invisible here. That
 * is not a gap: pre-commit runs its hooks after staging, and CI judges commits,
 * so every path that reaches this gate for real is tracked. Modified files need
 * no staging, and are deliberately left unstaged below to prove the ratchet does
 * not depend on it.
 */
function stage(dir: string) {
  git(dir, ["add", "-A"]);
}

/**
 * A repo whose `main` already contains `LEGACY_SPEC`, with `origin/main`
 * pointing at it — the shape `resolveBase` expects — and HEAD on a branch off
 * that commit, so working-tree edits read as this change's own.
 */
function repoWithLegacySleep() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-sleep-ratchet-"));
  tmpRoots.push(dir);
  git(dir, ["init", "-q", "-b", "main"]);
  write(dir, SPEC, LEGACY_SPEC);
  git(dir, ["add", "-A"]);
  git(dir, ["commit", "-q", "-m", "base"]);
  // `origin/main` without a remote: a local ref under refs/remotes is all
  // `merge-base origin/main HEAD` needs.
  git(dir, ["update-ref", "refs/remotes/origin/main", "HEAD"]);
  git(dir, ["checkout", "-q", "-b", "feature"]);
  return dir;
}

const run = (dir: string) =>
  findNewSleeps({
    repoRoot: dir,
    webDir: path.join(dir, "apps/web"),
    configFile: CONFIG_FILE,
  }) as Promise<{
    violations: Array<{ repoPath: string; line: number; whole: boolean; messageId: string }>;
    added: string[];
    modified: string[];
  }>;

describe("e2e sleep ratchet", () => {
  it("FAILS on a waitForTimeout added on a new line", async () => {
    const dir = repoWithLegacySleep();
    write(
      dir,
      SPEC,
      LEGACY_SPEC.replace(
        ANCHOR,
        '  await page.waitForTimeout(250);\n  await page.click("#done");',
      ),
    );

    const { violations } = await run(dir);
    expect(violations).toHaveLength(1);
    expect(violations[0]).toMatchObject({ line: 6, whole: false, messageId: "waitForTimeout" });
  });

  it("FAILS on a raw promise sleep added on a new line", async () => {
    const dir = repoWithLegacySleep();
    write(
      dir,
      SPEC,
      LEGACY_SPEC.replace(
        ANCHOR,
        '  await new Promise((r) => setTimeout(r, 250));\n  await page.click("#done");',
      ),
    );

    const { violations } = await run(dir);
    expect(violations).toHaveLength(1);
    expect(violations[0]).toMatchObject({ line: 6, whole: false, messageId: "promiseSleep" });
  });

  /**
   * The property that makes this mergeable at all: touching a line next to
   * somebody else's sleep must not make it yours. Without it the ratchet is a
   * migration treadmill and the first PR to edit a legacy spec is blocked.
   */
  it("PASSES when a line is edited right next to an existing untouched sleep", async () => {
    const dir = repoWithLegacySleep();
    write(
      dir,
      SPEC,
      LEGACY_SPEC.replace('  await page.goto("/");', '  await page.goto("/tasks");'),
    );

    const { modified, violations } = await run(dir);
    expect(modified).toEqual([SPEC]);
    expect(violations).toEqual([]);
  });

  it("PASSES on an existing untouched sleep with no changes at all", async () => {
    const { modified, added, violations } = await run(repoWithLegacySleep());
    expect(added).toEqual([]);
    expect(modified).toEqual([]);
    expect(violations).toEqual([]);
  });

  it("PASSES on the sanctioned dwell() and injectLatency() forms", async () => {
    const dir = repoWithLegacySleep();
    write(
      dir,
      SPEC,
      LEGACY_SPEC.replace(
        ANCHOR,
        [
          '  await dwell(page, 300, "library-timer", "Radix open delay publishes nothing");',
          '  await dwell(300, "poll-interval", "backend health poll; no page here");',
          '  await injectLatency(800, "make the in-flight spinner observable");',
          ANCHOR,
        ].join("\n"),
      ),
    );

    const { violations } = await run(dir);
    expect(violations).toEqual([]);
  });
});

/**
 * File-level selection: which paths the gate judges whole, which it judges by
 * line, and which it does not judge at all.
 */
describe("e2e sleep ratchet — file selection", () => {
  /** An added file carries no legacy debt, so it is judged whole. */
  it("FAILS on a sleep anywhere in a newly ADDED file", async () => {
    const dir = repoWithLegacySleep();
    write(dir, "apps/web/e2e/tests/demo/new.spec.ts", LEGACY_SPEC);
    stage(dir);

    const { added, violations } = await run(dir);
    expect(added).toEqual(["apps/web/e2e/tests/demo/new.spec.ts"]);
    expect(violations).toHaveLength(1);
    expect(violations[0]).toMatchObject({ line: 5, whole: true });
  });

  /**
   * A `git mv` of a legacy spec adds no lines, so it must report nothing.
   * Judging a rename as an addition would demand a full conversion for a move.
   */
  it("PASSES on a pure rename of a file full of legacy sleeps", async () => {
    const dir = repoWithLegacySleep();
    git(dir, ["mv", SPEC, "apps/web/e2e/tests/demo/renamed.spec.ts"]);

    const { violations } = await run(dir);
    expect(violations).toEqual([]);
  });

  /**
   * `.tsx` is selected by `isE2eCandidate`, so the eslint config must match it
   * too. It did not: the file was linted against no config at all, and
   * `warnIgnored: false` suppressed the only signal that would have shown it, so
   * the gate PASSED on a file full of sleeps. Found in review; this is the
   * regression test, and it fails against a `files: ["**\/*.ts"]` config.
   */
  it("FAILS on a sleep in a .tsx file, which the selector also picks up", async () => {
    const dir = repoWithLegacySleep();
    write(dir, "apps/web/e2e/fixtures/probe-component.tsx", LEGACY_SPEC);
    stage(dir);

    const { added, violations } = await run(dir);
    expect(added).toEqual(["apps/web/e2e/fixtures/probe-component.tsx"]);
    expect(violations).toHaveLength(1);
    expect(violations[0]).toMatchObject({ line: 5, whole: true, messageId: "waitForTimeout" });
  });

  /**
   * The finding must land on the changed line. Reporting the whole
   * `CallExpression` put it on the receiver, so converting `.click(…)` to
   * `.waitForTimeout(…)` in a wrapped chain left the finding on an UNCHANGED
   * line and the ratchet dropped it. Found in review.
   */
  it("FAILS when only the method line of a wrapped call changes", async () => {
    const dir = repoWithLegacySleep();
    const wrapped = [
      'import { test } from "@playwright/test";',
      "",
      'test("wrapped", async ({ page }) => {',
      "  await page",
      '    .click("#a");',
      "});",
      "",
    ].join("\n");
    write(dir, SPEC, wrapped);
    git(dir, ["add", "-A"]);
    git(dir, ["commit", "-q", "-m", "wrapped chain"]);
    git(dir, ["update-ref", "refs/remotes/origin/main", "HEAD"]);
    // Only line 5 changes; line 4 (`await page`) is untouched.
    write(dir, SPEC, wrapped.replace('    .click("#a");', "    .waitForTimeout(500);"));

    const { violations } = await run(dir);
    expect(violations).toHaveLength(1);
    expect(violations[0]).toMatchObject({ line: 5, whole: false, messageId: "waitForTimeout" });
  });

  /** Non-e2e changes are none of this gate's business. */
  it("ignores changes outside apps/web/e2e", async () => {
    const dir = repoWithLegacySleep();
    write(dir, "apps/web/components/card.tsx", "export const C = () => null;\n");
    write(dir, "apps/backend/main.go", "package main\n");

    const { added, modified, violations } = await run(dir);
    expect(added).toEqual([]);
    expect(modified).toEqual([]);
    expect(violations).toEqual([]);
  });

  /**
   * `causal-waits.ts` implements both banned forms. If the exemption ever stops
   * applying, the helper the whole effort promotes becomes unlandable.
   */
  it("exempts the causal-waits helper even when added whole", async () => {
    const dir = repoWithLegacySleep();
    write(
      dir,
      "apps/web/e2e/helpers/causal-waits.ts",
      [
        "export function dwell(page: Page, ms: number): Promise<void> {",
        "  return page.waitForTimeout(ms);",
        "}",
        "export function injectLatency(ms: number): Promise<void> {",
        "  return new Promise((resolve) => setTimeout(resolve, ms));",
        "}",
        "",
      ].join("\n"),
    );
    stage(dir);

    const { added, violations } = await run(dir);
    expect(added).toEqual(["apps/web/e2e/helpers/causal-waits.ts"]);
    expect(violations).toEqual([]);
  });
});
