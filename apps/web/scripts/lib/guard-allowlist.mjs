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
