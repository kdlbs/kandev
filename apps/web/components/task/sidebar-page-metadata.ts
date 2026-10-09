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
    if (entry.kind !== "task") continue;
    applyTaskMetadata(entry, maps);
  }
}

function applyTaskMetadata(
  entry: SidebarTaskPageResponse["entries"][number],
  maps: Parameters<typeof applySidebarPageMetadata>[1],
) {
  const workflowId = entry.workflow_id ?? entry.task?.workflow_id;
  const stepId = entry.workflow_step_id ?? entry.task?.workflow_step_id;
  if (entry.workflow_name && workflowId) {
    maps.workflowNameById.set(workflowId, entry.workflow_name);
  }
  if (entry.workflow_step_name && stepId) {
    maps.stepTitleById.set(stepId, entry.workflow_step_name);
  }
  if (entry.workflow_step_color && stepId) {
    maps.stepColorById.set(stepId, entry.workflow_step_color);
  }
}
