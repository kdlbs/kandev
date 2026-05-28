"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { IconSquarePlus } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { NewTaskDialog } from "@/app/office/components/new-task-dialog";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

type AppSidebarNewTaskItemProps = {
  collapsed: boolean;
};

/**
 * "New Task" entry in the sidebar primary nav. Office mode opens the richer
 * "New issue" dialog (projects/assignees/stages); regular Kandev opens the
 * standard task-create dialog wired to the active workflow. The Office dialog
 * must stay behind the `office` flag so it never leaks into regular mode.
 */
export function AppSidebarNewTaskItem({ collapsed }: AppSidebarNewTaskItemProps) {
  const router = useRouter();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const steps = useAppStore((s) => s.kanban.steps);
  const officeEnabled = useFeature("office");
  const [open, setOpen] = useState(false);

  const handleCreated = (task: Task) => {
    router.push(linkToTask(task.id));
  };

  return (
    <>
      <AppSidebarNavItem
        icon={IconSquarePlus}
        label="New Task"
        onClick={() => setOpen(true)}
        collapsed={collapsed}
        disabled={!workspaceId}
      />
      {workspaceId &&
        (officeEnabled ? (
          <NewTaskDialog open={open} onOpenChange={setOpen} />
        ) : (
          <TaskCreateDialog
            open={open}
            onOpenChange={setOpen}
            mode="create"
            workspaceId={workspaceId}
            workflowId={workflowId}
            defaultStepId={steps[0]?.id ?? null}
            steps={steps}
            onSuccess={handleCreated}
          />
        ))}
    </>
  );
}
