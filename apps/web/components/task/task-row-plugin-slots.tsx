"use client";

import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { TaskRowMetadataSlotProps } from "@/lib/plugins/types";
import { cn } from "@/lib/utils";

/**
 * Plugin-agnostic, compact secondary metadata for dense task-listing rows.
 * The component owns its layout wrapper so an empty registry contributes no
 * DOM or spacing.
 */
export function TaskRowMetadata({
  taskId,
  workflowStepId,
  surface,
  className,
}: {
  taskId: string;
  workflowStepId: string | null;
  surface: TaskRowMetadataSlotProps["surface"];
  className?: string;
}) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const registry = usePluginRegistry();
  if (registry.getSlotRegistrations("task-row-metadata").length === 0) return null;

  const slotProps: TaskRowMetadataSlotProps = { taskId, workspaceId, workflowStepId, surface };
  return (
    <div data-testid="task-row-metadata" className={cn("min-w-0", className)}>
      <PluginSlot name="task-row-metadata" slotProps={slotProps} />
    </div>
  );
}
