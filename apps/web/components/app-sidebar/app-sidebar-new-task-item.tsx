"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { IconSquarePlus, IconSubtask } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useInOffice } from "@/hooks/use-in-office";
import { TaskCreateDialog } from "@/components/task-create-dialog";

// The Office "New issue" dialog only renders on `/office` routes, but this item
// lives in the global sidebar (every page). Lazy-load it so its office-only
// dependencies aren't shipped in the bundle for non-office routes.
const NewTaskDialog = dynamic(
  () => import("@/app/office/components/new-task-dialog").then((m) => m.NewTaskDialog),
  { ssr: false },
);
import { NewSubtaskDialog } from "@/components/task/new-subtask-dialog";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

type AppSidebarNewTaskItemProps = {
  collapsed: boolean;
};

/**
 * "New Task" entry in the sidebar primary nav. Inside Office (an `/office`
 * route) it opens the richer "New issue" dialog (projects/assignees/stages);
 * everywhere else — including regular Kanban while the office feature is merely
 * enabled — it opens the standard task-create dialog wired to the active
 * workflow. Gate on `useInOffice()` (route), not the bare `office` flag, so the
 * Office dialog never leaks into Kanban mode.
 *
 * When the user is inside a task (an active task in regular mode), a trailing
 * subtask affordance appears so a child task can be created against the current
 * one — restoring the contextual action the retired dockview header dropdown
 * used to provide.
 */
export function AppSidebarNewTaskItem({ collapsed }: AppSidebarNewTaskItemProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const steps = useAppStore((s) => s.kanban.steps);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const activeTaskTitle = useAppStore((s) => {
    const id = s.tasks.activeTaskId;
    if (!id) return "";
    return s.kanban.tasks.find((t) => t.id === id)?.title ?? "";
  });
  const inOffice = useInOffice();
  const [open, setOpen] = useState(false);
  const [subtaskOpen, setSubtaskOpen] = useState(false);

  // The subtask affordance is available in both modes (office uses the richer
  // dialog for the primary New Task, but subtasks always go through the compact
  // NewSubtaskDialog, matching the retired dropdown). It needs an active task
  // and the expanded rail to host the trailing button.
  const canCreateSubtask = !collapsed && !!workspaceId && !!activeTaskId;

  return (
    <>
      <div className="relative">
        <AppSidebarNavItem
          icon={IconSquarePlus}
          label="New Task"
          onClick={() => setOpen(true)}
          collapsed={collapsed}
          disabled={!workspaceId}
          testId="create-task-button"
        />
        {canCreateSubtask && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                onClick={() => setSubtaskOpen(true)}
                aria-label="New subtask of current task"
                data-testid="sidebar-new-subtask"
                className="absolute right-1.5 top-1/2 -translate-y-1/2 flex h-6 w-6 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted hover:text-foreground cursor-pointer"
              >
                <IconSubtask className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">New subtask of current task</TooltipContent>
          </Tooltip>
        )}
      </div>
      {workspaceId &&
        (inOffice ? (
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
            onSuccess={() => setOpen(false)}
          />
        ))}
      {canCreateSubtask && (
        <NewSubtaskDialog
          open={subtaskOpen}
          onOpenChange={setSubtaskOpen}
          parentTaskId={activeTaskId}
          parentTaskTitle={activeTaskTitle}
        />
      )}
    </>
  );
}
