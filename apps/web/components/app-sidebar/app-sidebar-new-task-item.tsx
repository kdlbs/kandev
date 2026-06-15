"use client";

import { useCallback, useState } from "react";
import dynamic from "@/lib/routing/client-dynamic";
import { useRouter } from "@/lib/routing/client-router";
import { IconMessageCircle, IconSquarePlus, IconSubtask } from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useInOffice } from "@/hooks/use-in-office";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";

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

const ONE_ROW_ACTION_INSET_CLASS = "pr-10";
const TWO_ROW_ACTIONS_INSET_CLASS = "pr-16";

type RowActionButtonProps = {
  icon: TablerIcon;
  label: string;
  testId: string;
  onClick: () => void;
};

function RowActionButton({ icon: Icon, label, testId, onClick }: RowActionButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onClick}
          aria-label={label}
          data-testid={testId}
          className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted hover:text-foreground cursor-pointer"
        >
          <Icon className="h-3.5 w-3.5" />
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

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
  const router = useRouter();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const steps = useAppStore((s) => s.kanban.steps);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const activeTaskTitle = useAppStore((s) => {
    const id = s.tasks.activeTaskId;
    if (!id) return "";
    return s.kanban.tasks.find((t) => t.id === id)?.title ?? "";
  });
  const inOffice = useInOffice();
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const [open, setOpen] = useState(false);
  const [subtaskOpen, setSubtaskOpen] = useState(false);

  // The subtask affordance is available in both modes (office uses the richer
  // dialog for the primary New Task, but subtasks always go through the compact
  // NewSubtaskDialog, matching the retired dropdown). It needs an active task
  // and the expanded rail to host the trailing button.
  const canOpenQuickChat = !collapsed && !!workspaceId;
  const canCreateSubtask = !collapsed && !!workspaceId && !!activeTaskId;
  let actionInsetClass: string | undefined;
  // Keep the label clear of the absolute action cluster:
  // pr-10 covers one w-6 button + right-1.5 inset; pr-16 covers two buttons + gap-1.
  if (canCreateSubtask) {
    actionInsetClass = TWO_ROW_ACTIONS_INSET_CLASS;
  } else if (canOpenQuickChat) {
    actionInsetClass = ONE_ROW_ACTION_INSET_CLASS;
  }
  const handleRegularTaskCreated = useCallback(
    (
      task: Task,
      _mode: "create" | "edit",
      meta?: { taskSessionId?: string | null; willNavigate?: boolean },
    ) => {
      setOpen(false);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
      } else {
        setActiveTask(task.id);
      }
      if (meta?.willNavigate) return;
      router.push(linkToTask(task.id));
    },
    [router, setActiveSession, setActiveTask],
  );

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
          className={actionInsetClass}
        />
        {canOpenQuickChat && (
          <div className="absolute right-1.5 top-1/2 -translate-y-1/2 flex items-center gap-1 sidebar-fade-in">
            <RowActionButton
              icon={IconMessageCircle}
              label="Quick Chat"
              testId="sidebar-quick-chat-shortcut"
              onClick={handleOpenQuickChat}
            />
            {canCreateSubtask && (
              <RowActionButton
                icon={IconSubtask}
                label="New subtask of current task"
                testId="sidebar-new-subtask"
                onClick={() => setSubtaskOpen(true)}
              />
            )}
          </div>
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
            onSuccess={handleRegularTaskCreated}
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
