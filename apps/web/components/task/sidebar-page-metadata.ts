import type { SidebarTaskPageResponse } from "@/lib/types/http";

export function applySidebarPageMetadata(
  entries: SidebarTaskPageResponse["entries"],
  maps: {
    titleById: Map<string, string>;
    workflowNameById: Map<string, string>;
    stepTitleById: Map<string, string>;
    stepColorById: Map<string, string | null | undefined>;
  },
): void {
  for (const entry of entries) {
    if (entry.kind === "continuation" && entry.parent_id && entry.parent_title) {
      maps.titleById.set(entry.parent_id, entry.parent_title);
      continue;
    }
    if (entry.kind !== "task" || !entry.task) continue;
    if (entry.workflow_name && entry.task.workflow_id) {
      maps.workflowNameById.set(entry.task.workflow_id, entry.workflow_name);
    }
    if (entry.workflow_step_name && entry.task.workflow_step_id) {
      maps.stepTitleById.set(entry.task.workflow_step_id, entry.workflow_step_name);
    }
    if (entry.workflow_step_color && entry.task.workflow_step_id) {
      maps.stepColorById.set(entry.task.workflow_step_id, entry.workflow_step_color);
    }
  }
}
