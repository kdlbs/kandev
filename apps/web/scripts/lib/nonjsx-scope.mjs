/**
 * What `check-nonjsx-copy.mjs` scans by default: the whole `apps/web` source
 * tree, minus the trees below.
 *
 * Kept here rather than inline in the script so it can be tested. The scope is
 * the thing this check is worth: it defaulted to `i18nGuardFiles` for a long
 * time, which left 1141 of 2564 source files judged by neither this nor the
 * eslint rule — and a shrunken scope and a clean run look identical from the
 * outside. `nonjsx-scope.test.ts` asserts the denominator directly.
 *
 * By EXCLUSION, not by allowlist, so a directory added tomorrow is covered the
 * day it is created and this hole cannot silently reopen.
 */
import path from "node:path";

export const DEFAULT_GLOBS = ["**/*.ts", "**/*.tsx"];

/**
 * Directories that hold no shipped user-facing copy.
 *
 * `e2e` and `scripts` are tooling; `src/locales` IS the catalog; `dist` and
 * `node_modules` are build output.
 */
export const EXCLUDED_DIRS = [
  "node_modules",
  "dist",
  "e2e",
  "scripts",
  path.join("src", "locales"),
];

/**
 * Files that build fixtures out of literal strings on purpose.
 *
 * `*.test-helpers.ts` / `*.test-utils.ts` are imported only by tests and are the
 * same class as `*.test.ts` — guarding them would force every `title: "Test"` in
 * a fixture through the catalog.
 */
export const EXCLUDED_FILES =
  /(\.(test|spec|d)\.tsx?|\.test-(helpers|utils)\.tsx?|\.spec-helpers\.tsx?)$/;

/** Whether a repo-relative path is outside the scanned scope. */
export function isExcluded(file) {
  if (EXCLUDED_FILES.test(file)) return true;
  return EXCLUDED_DIRS.some((dir) => file === dir || file.startsWith(`${dir}${path.sep}`));
}

/**
 * The scanned set for a list of globs, or the default scope when none is given.
 *
 * @param {{globSync: (pattern: string, options: {cwd: string}) => Iterable<string>}} fs
 * @param {string} cwd
 * @param {string[]} [patterns]
 */
export function collectScannedFiles(fs, cwd, patterns = []) {
  const globs = patterns.length ? patterns : DEFAULT_GLOBS;
  const files = new Set();
  for (const glob of globs) {
    for (const match of fs.globSync(glob, { cwd })) {
      if (!/\.tsx?$/.test(match)) continue;
      if (isExcluded(match)) continue;
      files.add(match);
    }
  }
  return files;
}
