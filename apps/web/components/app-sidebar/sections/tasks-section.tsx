"use client";

import { IconCircleDot } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { TaskSessionSidebar } from "@/components/task/task-session-sidebar";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type TasksSectionProps = {
  collapsed: boolean;
};

/**
 * Wraps the workspace task list (formerly rendered as a dockview pane) inside
 * the unified AppSidebar Tasks section.
 */
export function TasksSection({ collapsed }: TasksSectionProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.tasks}
      label="Tasks"
      collapsed={collapsed}
      icon={IconCircleDot}
    >
      <div className="h-[420px] min-h-0 -mx-2.5 sidebar-fade-in-2">
        <TaskSessionSidebar workspaceId={workspaceId} workflowId={workflowId} />
      </div>
    </AppSidebarSection>
  );
}
