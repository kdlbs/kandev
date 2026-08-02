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
import path from "node:path";

import { ESLint } from "eslint";

import { lineInRanges, parseAddedLineRanges } from "./lib/diff-ranges.mjs";
import { git, REPO_ROOT, resolveBase, toPosixPath, WEB_DIR } from "./lib/git-base.mjs";

const WEB_PREFIX = "apps/web/";

/** Only UI source. Tests build fixtures out of literals on purpose. */
function isCandidate(repoPath) {
  if (!repoPath.startsWith(WEB_PREFIX)) return false;
  const rel = repoPath.slice(WEB_PREFIX.length);
  if (!/\.tsx?$/.test(rel)) return false;
  if (/\.(test|spec)\.tsx?$/.test(rel)) return false;
  if (rel.startsWith("e2e/") || rel.startsWith("scripts/")) return false;
  return /^(components|app|hooks|lib|src)\//.test(rel);
}

function changedFiles(base, filter) {
  // --find-renames explicitly: rename detection defaults on since Git 2.9, but a
  // repo with diff.renames=false would report a moved file as ADDED, and an
  // added file is judged whole — demanding a full migration for a plain git mv.
  return git(["diff", "--name-only", "--find-renames", `--diff-filter=${filter}`, base])
    .split("\n")
    .map((line) => line.trim())
    .filter(isCandidate);
}

const resolved = resolveBase();
if (resolved.skip) {
  console.log(`⚠ i18n new-code ratchet skipped — ${resolved.skip}`);
  process.exit(0);
}
const { base } = resolved;

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
  const repoPath = toPosixPath(path.relative(REPO_ROOT, result.filePath));
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
