import type { TaskSwitcherItem } from "./task-switcher-types";

/** Shared single-task eligibility for sidebar and palette entry points. */
export function taskRowActionAvailability(task: TaskSwitcherItem) {
  return {
    mark: !task.isArchived,
    edit: !task.isArchived && Boolean(task.workflowId && task.workflowStepId),
    nest: !task.isArchived && Boolean(task.workflowId),
    move: !task.isArchived && !task.isFromOffice && Boolean(task.workflowId),
    detach: Boolean(task.parentTaskId),
  };
}
