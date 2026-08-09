/**
 * Shape checks on the i18n guard allowlist itself.
 *
 * Split out of `check-guard-allowlist.mjs` so they can be tested directly: that
 * script resolves this repository's own git history and options module from
 * fixed paths, so it cannot be pointed at a fixture.
 */

/**
 * Entries listed more than once, each named once however often it repeats.
 *
 * Nothing else looks for one, because a duplicate changes no behaviour: ESLint is
 * indifferent to a repeated pattern, and the removal check in
 * `check-guard-allowlist.mjs` only ever inspects entries that LEFT the array. So
 * when #2214 added two, lint, `i18n:check`, the ratchet and the unit suite all
 * passed and it was caught by eye. What a duplicate damages is the record: a
 * second copy of a path an earlier migration already listed reads as the new
 * PR's own coverage, which is the misinformation the comments around the list
 * exist to prevent.
 *
 * EXACT duplicates only, deliberately. The list also carries entries a broader
 * glob already covers — `app/settings/system/storage/**` sits inside
 * `app/settings/system/**`, `system-page-shell.tsx` inside
 * `components/settings/system/*` — and #2202 kept those on purpose, as the
 * record of which PR migrated which half of a group. That redundancy is
 * intentional and must keep passing. Do not turn this into a subsumption check;
 * it would fight a decision the list makes knowingly.
 */
export function duplicateEntries(entries) {
  return [...new Set(entries.filter((entry, index) => entries.indexOf(entry) !== index))];
}

/**
 * Entries that match no file at all — listed, but guarding nothing.
 *
 * Both other checks ask what happened to an entry BETWEEN two states: the
 * removal check inspects entries that left the array, and `duplicateEntries`
 * compares the array with itself. Neither asks the prior question — does this
 * entry match anything? An entry that matched nothing on the day it was added
 * never leaves, is not a duplicate, and passes `pnpm lint` trivially because the
 * rule simply has no files to apply it to. The list grows, the ratchet reports
 * "N added", and the path is unguarded. It is the allowlist's own version of a
 * pass it had not earned: green everywhere, protecting nothing.
 *
 * A dynamic route is how you hit it, because Next-style directories are named in
 * glob syntax and `[id]` is a CHARACTER CLASS matching a single `i` or `d`:
 *
 *   app/settings/workspace/[id]/automations/**\/*.tsx      -> 0 files
 *   app/settings/workspace/[[]id[]]/automations/**\/*.tsx  -> 4 files
 *
 * #2247 shipped the unescaped form and review caught it; escaping it turned a
 * hardcoded literal in that route from 0 lint errors into 1. Three older entries
 * had the same defect. This makes the class impossible rather than the instances
 * fixed.
 *
 * `resolve` is injected so this is testable against a fixture: the calling
 * script resolves this repository's own paths from fixed locations and cannot be
 * pointed elsewhere.
 */
export function unmatchedEntries(entries, resolve) {
  return entries.filter((entry) => resolve(entry).length === 0);
}

/** An entry is a glob only if it carries glob metacharacters. */
const GLOB_METACHARACTERS = /[*?[\]{}]/;

function isFile(fsImpl, fullPath) {
  try {
    return fsImpl.statSync(fullPath).isFile();
  } catch {
    // Missing, or a broken symlink — either way it selects nothing.
    return false;
  }
}

/**
 * The files an allowlist entry currently matches, rooted at `cwd`.
 *
 * Exported so the removal check, the unmatched check and the tests share ONE
 * definition of "this entry has files" — they used to state it separately, and
 * two copies of a rule about glob semantics is how they drift apart. `fsImpl` is
 * a seam for tests; it takes `node:fs` in the script.
 *
 * **Directories do not count, and that is the whole subtlety.** ESLint flat
 * config matches `files` patterns against FILE paths, so an entry of
 * `components/foo` selects nothing — it does not stand for `components/foo/**`.
 * `existsSync` would happily confirm the directory exists and report the entry as
 * live, which would let exactly the born-dead entry this module exists to catch
 * through under a different spelling. Both branches are therefore filtered by
 * `isFile`: `globSync` returns directories too (`components/automations/*`
 * matches `.../trigger-configs`), so it is not only the literal case.
 */
export function filesForEntry(entry, { cwd, fsImpl }) {
  const candidates = GLOB_METACHARACTERS.test(entry) ? fsImpl.globSync(entry, { cwd }) : [entry];
  return candidates.filter((candidate) => isFile(fsImpl, `${cwd}/${candidate}`));
}
