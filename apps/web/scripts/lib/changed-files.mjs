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
 * Only Playwright E2E sources, for the sleep ratchet.
 *
 * The exact complement of the `e2e/` exclusion in {@link isCandidate}: the i18n
 * guard skips this tree because tests build fixtures out of literals on purpose,
 * and the sleep guard exists only for this tree. `.spec.ts` files are the
 * *primary* target here rather than an exclusion, which is why this cannot be
 * expressed as options on the other predicate.
 *
 * `causal-waits.ts` is not excluded here. Exemption belongs to the eslint config
 * (`SLEEP_EXEMPT_FILES`), so the editor, the preview and the gate all read one
 * list; filtering it out at selection time as well would be a second place to
 * keep in sync, and the one that fails silently.
 */
export function isE2eCandidate(repoPath) {
  if (!repoPath.startsWith(WEB_PREFIX)) return false;
  const rel = repoPath.slice(WEB_PREFIX.length);
  return rel.startsWith("e2e/") && /\.tsx?$/.test(rel);
}

/**
 * Every path matching `filter` (a `--diff-filter` value) between `base` and the
 * working tree, unfiltered.
 *
 * `--find-renames` explicitly: rename detection defaults on since Git 2.9, but a
 * repo with diff.renames=false would report a moved file as ADDED, and an added
 * file is judged whole — demanding a full migration for a plain `git mv`.
 */
export function changedPaths(base, filter, cwd) {
  return (
    git(["diff", "--name-only", "--find-renames", `--diff-filter=${filter}`, base], { cwd })
      .split("\n")
      .map((line) => line.trim())
      // `"".split("\n")` is `[""]`, so a no-match diff would otherwise yield one
      // empty "path". Both predicates reject it, but this function is exported and
      // its next caller should not have to know that.
      .filter(Boolean)
  );
}

/**
 * Candidate UI-source paths matching `filter`.
 *
 * A pure delegation to {@link changedPaths} + {@link isCandidate}: the i18n
 * ratchet's file selection is byte-for-byte what it was before the sleep ratchet
 * needed the same git invocation with a different predicate. Kept as its own
 * export rather than inlined at the call site so that stays true by
 * construction.
 */
export function changedFiles(base, filter, cwd) {
  return changedPaths(base, filter, cwd).filter(isCandidate);
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
