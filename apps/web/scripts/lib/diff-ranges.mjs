/**
 * Parse `git diff --unified=0` into the set of line numbers a change ADDED, so a
 * lint finding can be attributed to the change that introduced it rather than to
 * the file it happens to live in.
 *
 * This is what lets the i18n literal guard apply repo-wide to new code without
 * demanding that the surrounding legacy file be migrated first — the same model
 * as `golangci-lint --new-from-rev`, which this repo already uses for Go.
 */

/** Hunk header: `@@ -oldStart[,oldCount] +newStart[,newCount] @@ optional context` */
const HUNK = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@/;

/**
 * Map of repo-relative path -> array of inclusive `[start, end]` line ranges that
 * the diff added, in the NEW file's numbering.
 *
 * Deleted files and pure-deletion hunks contribute nothing: there is no new line
 * to attribute a finding to.
 *
 * @param {string} diffText output of `git diff --unified=0 <base>...<head>`
 * @returns {Map<string, Array<[number, number]>>}
 */
export function parseAddedLineRanges(diffText) {
  const ranges = new Map();
  let current = null;

  for (const line of diffText.split("\n")) {
    // Every file entry starts here, so reset first. Entries that emit no `+++`
    // header at all — a 100%-similarity rename, a binary file, a mode-only
    // change — would otherwise leave the PREVIOUS file selected, and any hunk
    // that followed would be attributed to it. Not reachable with today's git
    // output, but it is a one-line invariant instead of a standing assumption.
    if (line.startsWith("diff --git ")) {
      current = null;
      continue;
    }
    if (line.startsWith("+++ ")) {
      const target = line.slice(4).trim();
      // `+++ /dev/null` marks a deletion; `+++ b/path` is the new file.
      current = target === "/dev/null" ? null : target.replace(/^b\//, "");
      if (current && !ranges.has(current)) ranges.set(current, []);
      continue;
    }
    if (!current || !line.startsWith("@@")) continue;

    const match = HUNK.exec(line);
    if (!match) continue;
    const start = Number(match[1]);
    // A missing count means exactly one line; an explicit 0 means deletion-only.
    const count = match[2] === undefined ? 1 : Number(match[2]);
    if (count > 0) ranges.get(current).push([start, start + count - 1]);
  }

  return ranges;
}

/**
 * Whether `line` falls inside any of `ranges`.
 *
 * @param {Array<[number, number]>} ranges
 * @param {number} line
 */
export function lineInRanges(ranges, line) {
  if (!ranges) return false;
  return ranges.some(([start, end]) => line >= start && line <= end);
}
