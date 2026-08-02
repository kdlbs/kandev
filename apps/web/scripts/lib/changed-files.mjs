/**
 * Which files a change added or modified, and the raw diff its added-line ranges
 * are parsed from.
 *
 * Split out of `check-new-code-i18n.mjs` so the selection can be tested against
 * real git topologies — a merge ref, a rebased branch, a push — without needing
 * ESLint or this repository's own history.
 */
import { git } from "./git-base.mjs";

export const WEB_PREFIX = "apps/web/";

/** Only UI source. Tests build fixtures out of literals on purpose. */
export function isCandidate(repoPath) {
  if (!repoPath.startsWith(WEB_PREFIX)) return false;
  const rel = repoPath.slice(WEB_PREFIX.length);
  if (!/\.tsx?$/.test(rel)) return false;
  if (/\.(test|spec)\.tsx?$/.test(rel)) return false;
  if (rel.startsWith("e2e/") || rel.startsWith("scripts/")) return false;
  return /^(components|app|hooks|lib|src)\//.test(rel);
}

/**
 * Candidate paths matching `filter` (a `--diff-filter` value) between `base` and
 * the working tree.
 *
 * `--find-renames` explicitly: rename detection defaults on since Git 2.9, but a
 * repo with diff.renames=false would report a moved file as ADDED, and an added
 * file is judged whole — demanding a full migration for a plain `git mv`.
 */
export function changedFiles(base, filter, cwd) {
  return git(["diff", "--name-only", "--find-renames", `--diff-filter=${filter}`, base], { cwd })
    .split("\n")
    .map((line) => line.trim())
    .filter(isCandidate);
}

/**
 * The zero-context diff that added-line attribution is parsed from.
 *
 * Scoped to apps/web rather than to the individual files, because narrowing the
 * pathspec to only the NEW path of a rename hides the matching deletion and git
 * then reports the whole file as added — which would demand a full migration for
 * a pure `git mv`. Keeping both sides in scope lets rename detection pair them,
 * so a pure rename yields no hunks and reports nothing.
 */
export function webDiff(base, cwd) {
  return git(["diff", "--unified=0", "--find-renames", base, "--", WEB_PREFIX], { cwd });
}
