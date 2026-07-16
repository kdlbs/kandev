// Value normalization + custom-entry visibility for the project
// repository picker.

/**
 * Canonical form for comparing repository entries: trimmed, forward
 * slashes, no trailing slash, case-insensitive. Repo values mix git
 * URLs, Windows paths, and POSIX paths, so comparisons must survive
 * `C:\repo\` vs `c:/repo`.
 */
export function normalizeRepoValue(value: string): string {
  return value.trim().replace(/\\/g, "/").replace(/\/+$/g, "").toLowerCase();
}

/**
 * Whether the picker should offer the free-form "Use <query>" row.
 *
 * Only an exact normalized match against an option or an
 * already-attached entry hides the row. A substring or prefix overlap
 * must NOT hide it: with `/work/app-old` in the suggestions, typing
 * `/work/app` has to stay addable as a literal path — selecting the
 * near-miss suggestion would attach the wrong repo.
 */
export function shouldShowCustomEntry(
  query: string,
  optionValues: string[],
  excludedValues: string[],
): boolean {
  const q = normalizeRepoValue(query);
  if (!q) return false;
  const taken = (v: string) => normalizeRepoValue(v) === q;
  return !excludedValues.some(taken) && !optionValues.some(taken);
}
