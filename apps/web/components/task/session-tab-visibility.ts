export function countVisibleSessionPanels(
  panels: ReadonlyArray<{ id: string }>,
  taskSessionIds: ReadonlySet<string>,
): number {
  return panels.filter((panel) => {
    if (!panel.id.startsWith("session:")) return false;
    return taskSessionIds.has(panel.id.slice("session:".length));
  }).length;
}
