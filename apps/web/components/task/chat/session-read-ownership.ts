/**
 * A chat panel reads a session only when it is shown for that session's own
 * task. A Dockview chat panel keeps its saved session across task switches, so
 * without this check a panel left on another task's session marks that task
 * read while the user looks at a different task.
 */
export function isSessionReadableInPanel(
  isVisible: boolean,
  sessionTaskId: string | null | undefined,
  shownTaskId: string | null,
): boolean {
  if (!isVisible) return false;
  return shownTaskId === null || sessionTaskId === shownTaskId;
}
