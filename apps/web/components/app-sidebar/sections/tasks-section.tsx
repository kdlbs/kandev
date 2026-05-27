"use client";

import { IconLayoutKanban } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { TaskSessionSidebar } from "@/components/task/task-session-sidebar";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type KanbanSectionProps = {
  collapsed: boolean;
};

/**
 * Workspace kanban task list embedded as the bottom-most AppSidebar section.
 * The wrapper resets the embedded panel's card chrome (background) so it
 * visually integrates with the AppSidebar instead of looking transplanted from
 * its old dockview pane. The container flex-grows to fill the remaining
 * sidebar height; AppSidebar gives it `flex-1 min-h-0`.
 */
export function TasksSection({ collapsed }: KanbanSectionProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.tasks}
      label="Kanban"
      collapsed={collapsed}
      icon={IconLayoutKanban}
      grow
    >
      <div className="flex-1 min-h-0 -mx-2.5 sidebar-fade-in-2 [&_[data-testid=task-sidebar]]:bg-transparent [&_[data-testid=task-sidebar-scroll]]:bg-transparent">
        <TaskSessionSidebar workspaceId={workspaceId} workflowId={workflowId} />
      </div>
    </AppSidebarSection>
  );
}
