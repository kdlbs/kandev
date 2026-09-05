// Values only, mirroring the pattern in `lib/tasks/tasks-list-options.ts`.
export const KANBAN_SORT_OPTIONS = [{ value: "created_desc" }, { value: "priority_desc" }] as const;

export type KanbanSort = (typeof KANBAN_SORT_OPTIONS)[number]["value"];

export const DEFAULT_KANBAN_SORT: KanbanSort = "created_desc";

// The only source of display copy for these options; resolved at render
// against the `kanban:` catalog.
export const KANBAN_SORT_LABEL_KEYS: Record<KanbanSort, string> = {
  created_desc: "kanban:boardSortCreatedDesc",
  priority_desc: "kanban:boardSortPriorityDesc",
};

/**
 * Trims surrounding whitespace, then resolves an unrecognized value to the
 * default rather than failing. The server's `NormalizeKanbanSort` already
 * trims; this is the client-side counterpart so both sides agree on
 * `" priority_desc"`.
 */
export function parseKanbanSort(value: string | null | undefined): KanbanSort {
  const trimmed = value?.trim();
  return KANBAN_SORT_OPTIONS.some((option) => option.value === trimmed)
    ? (trimmed as KanbanSort)
    : DEFAULT_KANBAN_SORT;
}
