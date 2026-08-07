"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
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
import { linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { subscribeNewTaskCreationRequests } from "@/lib/desktop/new-task-request";

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

function useNewTaskCreationRequest(
  workspaceId: string | null,
  setOpen: Dispatch<SetStateAction<boolean>>,
) {
  useEffect(() => {
    if (!workspaceId) return;
    return subscribeNewTaskCreationRequests(() => setOpen(true));
  }, [setOpen, workspaceId]);
}

const ROW_ACTION_INSET_CLASS = "pr-16";
type RowActionButtonProps = {
  icon: TablerIcon;
  label: string;
  testId: string;
  onClick: () => void;
};

function RowActionButton({ icon: Icon, label, testId, onClick }: RowActionButtonProps) {
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const hoveredRef = useRef(false);

  const handleTooltipOpenChange = (nextOpen: boolean) => {
    // Focus is restored to this action after Quick Terminal closes. Keep the
    // tooltip pointer-driven so that accessibility focus does not leave a
    // stale popover behind the dialog.
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
 * Task-specific actions live on each task row's context menu, keeping this
 * global navigation item focused on creating top-level tasks and quick chats.
 */
export function AppSidebarNewTaskItem({ collapsed }: AppSidebarNewTaskItemProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const steps = useAppStore((s) => s.kanban.steps);
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const inOffice = useInOffice();
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const handleOpenQuickTerminal = useQuickTerminalLauncher(workspaceId);
  const [open, setOpen] = useState(false);
  useNewTaskCreationRequest(workspaceId, setOpen);

  const canOpenRowActions = !collapsed && !!workspaceId;
  const actionInsetClass = canOpenRowActions ? ROW_ACTION_INSET_CLASS : undefined;
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
          label={t("sidebar:newTask")}
          onClick={() => setOpen(true)}
          collapsed={collapsed}
          disabled={!workspaceId}
          testId="create-task-button"
          className={actionInsetClass}
        />
        {canOpenRowActions && (
          <div className="absolute right-1.5 top-1/2 -translate-y-1/2 flex items-center gap-1 sidebar-fade-in">
            <RowActionButton
              icon={IconTerminal2}
              label={t("sidebar:quickTerminal")}
              testId="sidebar-quick-terminal-shortcut"
              onClick={handleOpenQuickTerminal}
            />
            <RowActionButton
              icon={IconMessageCircle}
              label={t("sidebar:quickChat")}
              testId="sidebar-quick-chat-shortcut"
              onClick={handleOpenQuickChat}
            />
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
    </>
  );
}
