export const KANBAN_VIEW_MODES = {
  kanban: { id: "kanban", storedValue: "" },
  graph2: { id: "graph2", storedValue: "graph2" },
  graph: { id: "graph", storedValue: "graph" },
} as const;

export type KanbanViewId = keyof typeof KANBAN_VIEW_MODES;

export function getEffectiveViewId(value: string, isMobile: boolean): KanbanViewId {
  if (isMobile) return KANBAN_VIEW_MODES.kanban.id;
  return (
    Object.values(KANBAN_VIEW_MODES).find((mode) => mode.storedValue === value)?.id ??
    KANBAN_VIEW_MODES.kanban.id
  );
}
