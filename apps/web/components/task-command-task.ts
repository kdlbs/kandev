import { toKanbanTask } from "@/lib/kanban/map-task";
import type { Task } from "@/lib/types/http";
import type { TaskSwitcherItem } from "./task/task-switcher-types";

export function taskCommandItemFromDetail(task: Task): TaskSwitcherItem {
  const mapped = toKanbanTask(task);
  return {
    id: mapped.id,
    title: mapped.title,
    description: mapped.description,
    workspaceId: mapped.workspaceId,
    workflowId: mapped.workflowId,
    workflowStepId: mapped.workflowStepId,
    priority: mapped.priority,
    state: mapped.state,
    origin: mapped.origin,
    isArchived: mapped.isArchived,
    isFromOffice: mapped.isFromOffice,
    parentTaskId: mapped.parentTaskId ?? undefined,
    repositoryLinks: task.repositories?.flatMap((repository) =>
      repository.repository_id
        ? [{ repository_id: repository.repository_id, position: repository.position }]
        : [],
    ),
    workspaceMode: mapped.workspaceMode,
    isPRReview: mapped.isPRReview,
    isIssueWatch: mapped.isIssueWatch,
    issueInfo:
      mapped.issueUrl && mapped.issueNumber
        ? { url: mapped.issueUrl, number: mapped.issueNumber }
        : undefined,
  };
}
