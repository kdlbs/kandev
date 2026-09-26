/**
 * Placeholder shown by the repository `<Select>` on the kanban display
 * dropdown (desktop) and the mobile menu sheet. Both surfaces render the same
 * three states, so they share one mapping rather than two copies that drift.
 *
 * Returns a catalog KEY, not copy: `t()` must run at render, never here — a
 * `t()` at module scope freezes at the boot locale and the pseudo-locale
 * cannot detect it.
 */
export function getRepositoryPlaceholderKey(loading: boolean, empty: boolean): string {
  if (loading) return "kanban:loadingRepositories";
  if (empty) return "kanban:noRepositories";
  return "kanban:selectRepository";
}
