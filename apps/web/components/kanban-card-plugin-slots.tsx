"use client";

import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";

/**
 * `task-card-indicators` slot (AC13): reads the active workspace from the
 * store itself, rather than threading `workspaceId` through the
 * KanbanCardFrame -> KanbanCardShell -> KanbanCardBody prop chain, since
 * `<PluginSlot/>` already renders nothing when the slot is empty (AC14).
 */
export function TaskCardIndicators({ task }: { task: Task }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  return (
    <PluginSlot
      name="task-card-indicators"
      slotProps={{ taskId: task.id, workspaceId, workflowStepId: task.workflowStepId }}
    />
  );
}

/**
 * `task-card-tags` slot: unlike `task-card-indicators` (cramped, inline
 * beside the title/PR icon), this renders in its own row on the card so a
 * contribution like a row of tag chips has room. Same `slotProps` shape as
 * `task-card-indicators` — `{ taskId, workspaceId, workflowStepId }`.
 */
export function TaskCardTags({ task }: { task: Task }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  return (
    <PluginSlot
      name="task-card-tags"
      slotProps={{ taskId: task.id, workspaceId, workflowStepId: task.workflowStepId }}
    />
  );
}
