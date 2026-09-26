export function clearSelectionBeforeAction(
  actingOnSelection: boolean,
  onClearSelection: (() => void) | undefined,
  handler?: (id: string) => void,
) {
  if (!actingOnSelection || !onClearSelection || !handler) return handler;
  return (id: string) => {
    onClearSelection();
    handler(id);
  };
}
