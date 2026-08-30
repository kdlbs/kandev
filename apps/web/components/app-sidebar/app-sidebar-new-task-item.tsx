"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import dynamic from "@/lib/routing/client-dynamic";
import { useRouter } from "@/lib/routing/client-router";
import { IconMessageCircle, IconSquarePlus, IconTerminal2 } from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useInOffice } from "@/hooks/use-in-office";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useQuickTerminalLauncher } from "@/hooks/use-quick-terminal-launcher";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { IMPROVE_KANDEV_WORKSPACE_NAME } from "@/components/improve-kandev-dialog-model";
import { linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { subscribeNewTaskCreationRequests } from "@/lib/desktop/new-task-request";
import type { QuickChatActivityState } from "@/lib/state/slices/ui/quick-chat-activity-selectors";
import { QuickChatActivityIndicator } from "@/components/quick-chat/quick-chat-activity-indicator";
import { useQuickChatActivity } from "@/components/quick-chat/use-quick-chat-activity";
import { AppSidebarWorkspaceActions } from "./app-sidebar-workspace-actions";

// The Office "New issue" dialog only renders on `/office` routes, but this item
// lives in the global sidebar (every page). Lazy-load it so its office-only
// dependencies aren't shipped in the bundle for non-office routes.
const NewTaskDialog = dynamic(
  () => import("@/app/office/components/new-task-dialog").then((m) => m.NewTaskDialog),
  { ssr: false },
);
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

type AppSidebarNewTaskItemProps = {
  collapsed: boolean;
};

function useNewTaskCreationRequest(workspaceId: string | null, openDialog: () => void) {
  useEffect(() => {
    if (!workspaceId) return;
    return subscribeNewTaskCreationRequests(openDialog);
  }, [workspaceId, openDialog]);
}

type RowActionButtonProps = {
  icon: TablerIcon;
  label: string;
  testId: string;
  activity?: QuickChatActivityState;
  onClick: () => void;
};

function RowActionButton({
  icon: Icon,
  label,
  testId,
  onClick,
  activity = null,
}: RowActionButtonProps) {
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const hoveredRef = useRef(false);
  const handleTooltipOpenChange = (nextOpen: boolean) => {
    if (nextOpen && !hoveredRef.current) return;
    setTooltipOpen(nextOpen);
  };

  return (
    <Tooltip open={tooltipOpen} onOpenChange={handleTooltipOpenChange}>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onClick}
          onPointerEnter={() => {
            hoveredRef.current = true;
            setTooltipOpen(true);
          }}
          onPointerLeave={() => {
            hoveredRef.current = false;
            setTooltipOpen(false);
          }}
          onFocus={() => {
            hoveredRef.current = false;
            setTooltipOpen(false);
          }}
          aria-label={label}
          data-testid={testId}
          className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted hover:text-foreground cursor-pointer"
        >
          <span className="relative flex">
            <Icon className="h-3.5 w-3.5" />
            <QuickChatActivityIndicator activity={activity} />
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

type RoutedNewTaskDialogProps = {
  workspaceId: string | null;
  inOffice: boolean;
  isImproveWorkspace: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workflowId: string | null;
  steps: Array<{ id: string; title: string }>;
  onRegularTaskCreated: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null; willNavigate?: boolean },
  ) => void;
};

/** Picks the dialog the New Task entry opens: Office on /office routes, the
 *  shared Improve Kandev dialog (hosted by the sidebar footer) inside the
 *  dedicated improve workspace, and the standard task-create dialog everywhere
 *  else. */
function RoutedNewTaskDialog({
  workspaceId,
  inOffice,
  isImproveWorkspace,
  open,
  onOpenChange,
  workflowId,
  steps,
  onRegularTaskCreated,
}: RoutedNewTaskDialogProps) {
  if (!workspaceId) return null;
  if (inOffice) {
    return <NewTaskDialog open={open} onOpenChange={onOpenChange} />;
  }
  // The improve workspace opens the shared Improve Kandev dialog via the store
  // flag; the footer renders the dialog itself, so nothing mounts here.
  if (isImproveWorkspace) {
    return null;
  }
  return (
    <TaskCreateDialog
      open={open}
      onOpenChange={onOpenChange}
      mode="create"
      workspaceId={workspaceId}
      workflowId={workflowId}
      defaultStepId={steps[0]?.id ?? null}
      steps={steps}
      onSuccess={onRegularTaskCreated}
    />
  );
}

/**
 * "New Task" entry in the sidebar primary nav. Inside Office (an `/office`
 * route) it opens the richer "New issue" dialog (projects/assignees/stages);
 * inside the dedicated "Improve Kandev" workspace it opens the Improve Kandev
 * dialog (same as the footer button — all tasks there are contribution tasks);
 * everywhere else — including regular Kanban while the office feature is merely
 * enabled — it opens the standard task-create dialog wired to the active
 * workflow. Gate on `useInOffice()` (route), not the bare `office` flag, so the
 * Office dialog never leaks into Kanban mode.
 *
 * Task-specific actions live on each task row's context menu, keeping this
 * global navigation item focused on creating top-level tasks and quick chats.
 */
export function AppSidebarNewTaskItem({ collapsed }: AppSidebarNewTaskItemProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const activeWorkspace = useAppStore((s) =>
    s.workspaces.items.find((workspace) => workspace.id === s.workspaces.activeId),
  );
  const setImproveDialogOpen = useAppStore((s) => s.setImproveDialogOpen);
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const steps = useAppStore((s) => s.kanban.steps);
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const inOffice = useInOffice();
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const { activity: quickChatActivity, label: quickChatLabel } = useQuickChatActivity(workspaceId);
  const handleOpenQuickTerminal = useQuickTerminalLauncher(workspaceId);
  const [open, setOpen] = useState(false);
  const isImproveWorkspace = activeWorkspace?.name === IMPROVE_KANDEV_WORKSPACE_NAME;

  const handleOpenNewTask = useCallback(() => {
    if (isImproveWorkspace) {
      setImproveDialogOpen(true);
    } else {
      setOpen(true);
    }
  }, [isImproveWorkspace, setImproveDialogOpen]);
  useNewTaskCreationRequest(workspaceId, handleOpenNewTask);

  const canOpenRowActions = !collapsed && !!workspaceId;
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
      <div className="flex min-w-0 items-center gap-1">
        <AppSidebarNavItem
          icon={IconSquarePlus}
          label={t("sidebar:newTask")}
          onClick={handleOpenNewTask}
          collapsed={collapsed}
          disabled={!workspaceId}
          testId="create-task-button"
          className={!collapsed ? "min-w-0 flex-1" : undefined}
        />
        {canOpenRowActions && (
          <div className="flex shrink-0 items-center gap-1 sidebar-fade-in">
            <RowActionButton
              icon={IconTerminal2}
              label={t("sidebar:quickTerminal")}
              testId="sidebar-quick-terminal-shortcut"
              onClick={handleOpenQuickTerminal}
            />
            <RowActionButton
              icon={IconMessageCircle}
              label={quickChatLabel}
              testId="sidebar-quick-chat-shortcut"
              onClick={handleOpenQuickChat}
              activity={quickChatActivity}
            />
            <AppSidebarWorkspaceActions
              workspaceId={workspaceId}
              workspaceLabel={activeWorkspace?.name}
              presentation="desktop"
            />
          </div>
        )}
      </div>
      <RoutedNewTaskDialog
        workspaceId={workspaceId}
        inOffice={inOffice}
        isImproveWorkspace={isImproveWorkspace}
        open={open}
        onOpenChange={setOpen}
        workflowId={workflowId}
        steps={steps}
        onRegularTaskCreated={handleRegularTaskCreated}
      />
    </>
  );
}
