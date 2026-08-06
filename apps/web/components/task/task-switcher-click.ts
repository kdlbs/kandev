import type { KeyboardEvent, MouseEvent } from "react";

/**
 * Modifier-aware sidebar row click: cmd/ctrl toggles one task, shift extends a
 * range, a plain click toggles while a selection is active and otherwise
 * navigates to the task.
 */
export function dispatchSidebarRowClick(
  e: MouseEvent | KeyboardEvent,
  taskId: string,
  isSelecting: boolean,
  handlers: {
    onSelectTask: (taskId: string) => void;
    onToggleSelectTask?: (taskId: string) => void;
    onSelectTaskRange?: (taskId: string) => void;
  },
  isArchived = false,
): void {
  if (isArchived) {
    handlers.onSelectTask(taskId);
    return;
  }
  // Only intercept a modifier click when the matching handler is wired (the
  // mobile switcher renders without selection handlers — there a Cmd/Shift click
  // must still navigate rather than become a no-op).
  if ((e.metaKey || e.ctrlKey) && handlers.onToggleSelectTask) {
    e.preventDefault();
    handlers.onToggleSelectTask(taskId);
    return;
  }
  if (e.shiftKey && handlers.onSelectTaskRange) {
    e.preventDefault();
    handlers.onSelectTaskRange(taskId);
    return;
  }
  if (isSelecting && handlers.onToggleSelectTask) {
    handlers.onToggleSelectTask(taskId);
    return;
  }
  handlers.onSelectTask(taskId);
}
