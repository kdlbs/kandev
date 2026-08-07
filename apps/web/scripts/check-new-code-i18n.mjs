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
 * With no --base it uses `git merge-base HEAD origin/main`, which is the right
 * answer on every checkout shape — including CI's `refs/pull/N/merge`, where it
 * resolves to the merge ref's base-side parent. Prefer passing no --base. An
 * explicit one is floored at that same fork point (see lib/git-base.mjs), because
 * a base sha captured when a PR opened goes stale the moment main moves.
 */
import path from "node:path";

import { ESLint } from "eslint";

import { changedFiles, webDiff } from "./lib/changed-files.mjs";
import { lineInRanges, parseAddedLineRanges } from "./lib/diff-ranges.mjs";
import { REPO_ROOT, resolveBase, toPosixPath, WEB_DIR } from "./lib/git-base.mjs";

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
const addedRanges = modified.length === 0 ? new Map() : parseAddedLineRanges(webDiff(base));

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
