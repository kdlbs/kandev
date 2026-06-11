/**
 * Pick the first candidate id that exists in `items`, falling back to the
 * first item. Candidates are checked in the order given, so callers pass them
 * highest-priority first.
 *
 * Used by SSR to resolve the active workspace from (in priority order) a URL
 * `workspaceId` param, the `office-active-workspace` cookie set by the sidebar
 * picker, and the user's saved `workspace_id` setting.
 */
export function resolveActiveId<T extends { id: string }>(
  items: T[],
  ...preferredIds: (string | null | undefined)[]
): string | null {
  for (const id of preferredIds) {
    if (id == null) continue;
    const match = items.find((i) => i.id === id);
    if (match) return match.id;
  }
  return items[0]?.id ?? null;
}
