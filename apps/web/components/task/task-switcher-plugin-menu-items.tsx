"use client";

import { KanbanCardContextMenuItems } from "@/components/kanban-card-menu-items";
import { buildPrimaryPluginEntries } from "@/components/plugins/task-menu-actions";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import type { TaskSwitcherItem } from "./task-switcher-types";

export function TaskPluginPrimaryMenuItems({
  task,
  disabled,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
}) {
  usePluginRegistry();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { isMobile } = useResponsiveBreakpoint();
  if (!workspaceId) return null;

  const context: PluginTaskMenuContext = {
    workspaceId,
    taskId: task.id,
    taskTitle: task.title,
    workflowStepId: task.workflowStepId ?? null,
    presentation: isMobile ? "mobile" : "desktop",
  };
  return <KanbanCardContextMenuItems entries={buildPrimaryPluginEntries({ disabled, context })} />;
}
