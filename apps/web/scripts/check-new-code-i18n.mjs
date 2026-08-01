#!/usr/bin/env node
/**
 * Ratchet: NEW user-facing copy must go through `t()` / `<Trans>`, everywhere.
 *
 * The eslint guard in eslint.config.mjs is an error only on `i18nGuardFiles` —
 * paths already migrated — because a repo-wide error on a half-migrated codebase
 * breaks every unrelated PR that adds a label. That leaves a hole this closes:
 * a brand-new component under an un-migrated directory is currently unguarded.
 *
 * The fix is to judge the CHANGE rather than the FILE:
 *
 *   - a file the change ADDED must be clean outright. It carries no legacy debt,
 *     so requiring zero literals costs nothing.
 *   - a file the change MODIFIED is judged only on the lines it touched. Existing
 *     literals elsewhere in the file are somebody else's migration, not this PR's
 *     problem — which is exactly why this can be repo-wide without a treadmill.
 *
 * This mirrors `golangci-lint run --new-from-rev` in .pre-commit-config.yaml, so
 * it is the same contract the Go side already enforces.
 *
 * NOTE: this raises the floor, it does not seal the box. The rule only sees plain
 * literals and template literals in JSX — `confirm()` arguments and copy returned
 * from plain `.ts` helpers stay invisible. The pseudo-locale remains the
 * completeness check (docs/i18n.md).
 *
 * Usage:
 *   node scripts/check-new-code-i18n.mjs [--base <ref-or-sha>]
 *
 * With no --base it uses `git merge-base HEAD origin/main`. CI passes the PR's
 * base SHA explicitly, matching the Prettier step in frontend-tests.yml.
 */
import { execFileSync } from "node:child_process";
import path from "node:path";

import { ESLint } from "eslint";

import { lineInRanges, parseAddedLineRanges } from "./lib/diff-ranges.mjs";

const WEB_DIR = path.resolve(import.meta.dirname, "..");
const REPO_ROOT = path.resolve(WEB_DIR, "..", "..");
const WEB_PREFIX = "apps/web/";

/** Only UI source. Tests build fixtures out of literals on purpose. */
function isCandidate(repoPath) {
  if (!repoPath.startsWith(WEB_PREFIX)) return false;
  const rel = repoPath.slice(WEB_PREFIX.length);
  if (!/\.tsx?$/.test(rel)) return false;
  if (/\.test\.tsx?$/.test(rel)) return false;
  if (rel.startsWith("e2e/") || rel.startsWith("scripts/")) return false;
  return /^(components|app|hooks|lib|src)\//.test(rel);
}

function git(args) {
  return execFileSync("git", args, {
    cwd: REPO_ROOT,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
  });
}

/**
 * The fork point to judge against.
 *
 * Resolved to a merge-base so that commits landed on the base branch since this
 * one forked are not mistaken for this change's work. Everything downstream then
 * uses a TWO-dot diff from here to the WORKING TREE, not to HEAD, so the check
 * sees staged-but-uncommitted edits — that is what makes it usable from
 * pre-commit, and in CI the working tree equals HEAD so the result is identical.
 */
function resolveBase() {
  const flag = process.argv.indexOf("--base");
  const requested = flag !== -1 && process.argv[flag + 1] ? process.argv[flag + 1] : "origin/main";
  try {
    return git(["merge-base", requested, "HEAD"]).trim();
  } catch {
    return null;
  }
}

function changedFiles(base, filter) {
  return git(["diff", "--name-only", `--diff-filter=${filter}`, base])
    .split("\n")
    .map((line) => line.trim())
    .filter(isCandidate);
}

const base = resolveBase();
if (!base) {
  // A shallow clone with no origin/main and no --base: nothing to compare against.
  // Skip rather than fail, so the check can never block for an environment reason.
  console.log("⚠ i18n new-code ratchet skipped — no base ref (pass --base <sha>)");
  process.exit(0);
}

const added = changedFiles(base, "A");
// Renames and copies are judged like modifications, NOT like additions: moving a
// legacy file must not suddenly demand that its whole contents be migrated. A
// pure rename adds no lines and so reports nothing; a rename-plus-edit is judged
// on the edited lines, same as any other modification.
const modified = changedFiles(base, "MRC");
const targets = [...added, ...modified];

if (targets.length === 0) {
  console.log("✓ i18n new-code ratchet — no UI source added or modified");
  process.exit(0);
}

// Line attribution only matters for modified files; added files are judged whole.
//
// Scoped to apps/web rather than to the individual files, because narrowing the
// pathspec to only the NEW path of a rename hides the matching deletion and git
// then reports the whole file as added — which would demand a full migration for
// a pure `git mv`. Keeping both sides in scope lets rename detection pair them,
// so a pure rename yields no hunks and reports nothing.
const addedRanges =
  modified.length === 0
    ? new Map()
    : parseAddedLineRanges(git(["diff", "--unified=0", "--find-renames", base, "--", WEB_PREFIX]));

const eslint = new ESLint({
  cwd: WEB_DIR,
  overrideConfigFile: path.join(WEB_DIR, "eslint.i18n.config.mjs"),
});
const results = await eslint.lintFiles(targets.map((f) => path.join(REPO_ROOT, f)));

const addedSet = new Set(added);
const violations = [];
for (const result of results) {
  const repoPath = path.relative(REPO_ROOT, result.filePath);
  const whole = addedSet.has(repoPath);
  const ranges = addedRanges.get(repoPath);
  for (const message of result.messages) {
    if (message.ruleId !== "i18next/no-literal-string") continue;
    if (!whole && !lineInRanges(ranges, message.line)) continue;
    violations.push({ repoPath, whole, line: message.line, text: message.message });
  }
}

if (violations.length === 0) {
  console.log(
    `✓ i18n new-code ratchet — ${added.length} added + ${modified.length} modified file(s) clean`,
  );
  process.exit(0);
}

console.error(
  `\n✖ ${violations.length} hardcoded user-facing string(s) in new code.\n` +
    `  New copy must go through t() / <Trans> even in a directory that has not\n` +
    `  been migrated yet. See docs/i18n.md ("TL;DR for adding a string").\n`,
);
for (const violation of violations) {
  const scope = violation.whole ? "new file" : "changed line";
  console.error(`  ${violation.repoPath}:${violation.line}  [${scope}]  ${violation.text}`);
}
console.error("");
process.exit(1);
