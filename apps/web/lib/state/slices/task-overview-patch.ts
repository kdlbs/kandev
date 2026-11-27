import { toKanbanTask, type TaskLike } from "@/lib/kanban/map-task";
import type { TaskOverview, TaskOverviewPatch } from "./task-overview-types";

const renamedFields: Record<string, Array<keyof TaskOverview>> = {
  task_id: ["id"],
  parent_id: ["parentTaskId"],
  archived_at: ["isArchived"],
  repository_id: ["repositoryId"],
  repositories: ["repositories", "repositoryId"],
  metadata: ["metadata", "workspaceMode", "isPRReview", "isIssueWatch", "issueUrl", "issueNumber"],
};

/** Preserve wire omission separately from an explicit null or empty collection. */
export function taskOverviewPatch(source: TaskLike): TaskOverviewPatch {
  const mapped = toKanbanTask(source);
  const patch: TaskOverviewPatch = { id: mapped.id };
  for (const wireKey of Object.keys(source)) {
    const camelKey = wireKey.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());
    const fields = renamedFields[wireKey] ?? [camelKey as keyof TaskOverview];
    for (const field of fields) {
      if (Object.hasOwn(mapped, field)) Object.assign(patch, { [field]: mapped[field] });
    }
  }
  if (source.primary_session_id === null) {
    for (const field of Object.keys(mapped) as Array<keyof TaskOverview>) {
      if (field.startsWith("primary") || field === "isRemoteExecutor") {
        Object.assign(patch, { [field]: mapped[field] });
      }
    }
  }
  // Permission projections fail closed when an older publisher omits them.
  patch.runnerEditable = mapped.runnerEditable;
  patch.runnerIneligibleReason = mapped.runnerIneligibleReason;
  return patch;
}
