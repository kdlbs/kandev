import { isSessionDeletable } from "@/hooks/domains/session/session-action-state";
import type { TaskSessionState } from "@/lib/types/http";

/** Counts task-owned session panels, excluding stale sessions and other panel types. */
export function countVisibleSessionPanels(
  panels: ReadonlyArray<{ id: string }>,
  taskSessionIds: ReadonlySet<string>,
): number {
  return panels.filter((panel) => {
    if (!panel.id.startsWith("session:")) return false;
    return taskSessionIds.has(panel.id.slice("session:".length));
  }).length;
}

/** Keeps hide-only close independent of restart while guarding deletion and the last tab. */
export function shouldShowSessionTabCloseAction(
  behavior: "delete_session" | "hide_panel",
  sessionState: TaskSessionState | null,
  state: { sessionCount: number; visibleSessionCount: number; isRestarting: boolean },
): boolean {
  if (behavior === "hide_panel") return state.visibleSessionCount > 1;
  return (
    !state.isRestarting &&
    state.sessionCount > 1 &&
    !!sessionState &&
    isSessionDeletable(sessionState)
  );
}
