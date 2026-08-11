"use client";

import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";

/**
 * `task-row-tags` slot: the row-level analog of `task-card-tags` for the
 * denser task-listing surfaces (sidebar task tree, `/tasks` list). Reads the
 * active workspace from the store itself, same as `TaskCardTags` — the
 * caller doesn't need to thread `workspaceId` down, and `<PluginSlot/>`
 * already renders nothing when the slot is empty.
 */
export function TaskRowTags({
  taskId,
  workflowStepId,
  surface,
}: {
  taskId: string;
  workflowStepId: string | null;
  surface: "sidebar" | "task-list";
}) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  return (
    <PluginSlot name="task-row-tags" slotProps={{ taskId, workspaceId, workflowStepId, surface }} />
  );
}
