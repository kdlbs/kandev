"use client";

import { IconCircleDot } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { TaskSessionSidebar } from "@/components/task/task-session-sidebar";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";
import { TasksViewPicker } from "./tasks-view-picker";

type TasksSectionProps = {
  collapsed: boolean;
};

export function TasksSection({ collapsed }: TasksSectionProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.tasks}
      label="Tasks"
      collapsed={collapsed}
      icon={IconCircleDot}
      grow
    >
      <div className="shrink-0 px-2 pb-1">
        <TasksViewPicker />
      </div>
      <div className="flex-1 min-h-0 [&_[data-testid=task-sidebar]]:bg-transparent [&_[data-testid=task-sidebar-scroll]]:bg-transparent">
        <TaskSessionSidebar workspaceId={workspaceId} workflowId={workflowId} hideFilterBar />
      </div>
    </AppSidebarSection>
  );
}
