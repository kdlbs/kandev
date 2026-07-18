"use client";

import { useCallback, useMemo, useState, memo } from "react";
import { IconMessageCircle, IconPlus } from "@tabler/icons-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { TaskSwitcher } from "../task-switcher";
import type { TaskSwitcherItem } from "../task-switcher";
import { SidebarFilterBar } from "../sidebar-filter/sidebar-filter-bar";
import type { StepDef } from "../task-switcher-context-menu";
import type { TaskMoveWorkflow } from "../task-move-context-menu";
import { applyView } from "@/lib/sidebar/apply-view";
import { useAppStore } from "@/components/state-provider";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { WorkspaceSwitcher } from "../workspace-switcher";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { TaskArchiveConfirmDialog } from "../task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "../task-delete-confirm-dialog";
import { TaskDetachTargetConfirmDialog } from "../task-detach-confirm-dialog";
import { TaskRenameDialog } from "../task-rename-dialog";
import { SidebarLinkDialogs } from "../task-session-sidebar-dialogs";
import {
  useSidebarLinkActions,
  type SidebarLinkTarget,
} from "../task-session-sidebar-link-actions";
import { useSidebarTaskLinking } from "../task-session-sidebar-task-linking";
import { useSheetData, useSheetActions } from "./session-task-switcher-sheet-hooks";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useMobileTaskRename } from "./use-mobile-task-rename";

type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  presentation?: "sheet" | "drawer";
};

function useMobileTaskLinking(
  workspaceId: string | null,
  linkTaskById: ReadonlyMap<string, SidebarLinkTarget>,
) {
  const actions = useSidebarLinkActions(linkTaskById);
  const taskListHandlers = useSidebarTaskLinking(workspaceId, actions);
  const { repositories } = useRepositories(workspaceId);

  return {
    actions,
    repositories,
    taskListHandlers,
  };
}

export function MobileTaskList({
  tasks,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onDetachTask,
  onLinkPullRequest,
  onLinkIssue,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  deletingTaskId,
  isLoading,
}: {
  tasks: TaskSwitcherItem[];
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask: (taskId: string) => void;
  onDeleteTask: (taskId: string) => Promise<void> | void;
  onDetachTask: (taskId: string) => Promise<void> | void;
  onLinkPullRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkJiraTicket?: (taskId: string, taskTitle?: string) => void;
  onLinkLinearIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkSentryIssue?: (taskId: string, taskTitle?: string) => void;
  deletingTaskId: string | null;
  isLoading?: boolean;
}) {
  const view = useEffectiveSidebarView();
  const {
    pinnedTaskIds,
    orderedTaskIds,
    subtaskOrderByParentId,
    togglePinnedTask,
    handleReorderGroup,
    handleReorderSubtasks,
  } = useSidebarTaskPrefs();
  const toggleSidebarGroupCollapsed = useAppStore((s) => s.toggleSidebarGroupCollapsed);
  const collapsedSubtaskParents = useAppStore((s) => s.collapsedSubtaskParents);
  const toggleSubtaskCollapsed = useAppStore((s) => s.toggleSubtaskCollapsed);
  const handleToggleGroup = useCallback(
    (groupKey: string) => toggleSidebarGroupCollapsed(view.id, groupKey),
    [toggleSidebarGroupCollapsed, view.id],
  );
  const grouped = useMemo(
    () =>
      applyView(tasks, view, {
        pinnedTaskIds,
        orderedTaskIds,
        subtaskOrderByParentId,
      }),
    [tasks, view, pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId],
  );
  return (
    <TaskSwitcher
      grouped={grouped}
      workflows={workflows}
      stepsByWorkflowId={stepsByWorkflowId}
      activeTaskId={activeTaskId}
      selectedTaskId={selectedTaskId}
      collapsedGroupKeys={view.collapsedGroups}
      onToggleGroup={handleToggleGroup}
      collapsedSubtaskParentIds={collapsedSubtaskParents}
      onToggleSubtasks={toggleSubtaskCollapsed}
      onSelectTask={onSelectTask}
      onRenameTask={onRenameTask}
      onArchiveTask={onArchiveTask}
      onDeleteTask={onDeleteTask}
      onDetachTask={onDetachTask}
      onLinkPullRequest={onLinkPullRequest}
      onLinkIssue={onLinkIssue}
      onLinkJiraTicket={onLinkJiraTicket}
      onLinkLinearIssue={onLinkLinearIssue}
      onLinkSentryIssue={onLinkSentryIssue}
      onTogglePin={togglePinnedTask}
      onReorderGroup={handleReorderGroup}
      onReorderSubtasks={handleReorderSubtasks}
      pinnedTaskIds={pinnedTaskIds}
      deletingTaskId={deletingTaskId}
      isLoading={isLoading}
      totalTaskCount={tasks.length}
    />
  );
}

function TaskSwitcherSurfaceHeader({
  workspaceId,
  workspaces,
  onWorkspaceChange,
  onQuickChat,
  onNewTask,
  presentation,
}: {
  workspaceId: string | null;
  workspaces: Array<{ id: string; name: string }>;
  onWorkspaceChange: (workspaceId: string) => void;
  onQuickChat: () => void;
  onNewTask: () => void;
  presentation: "sheet" | "drawer";
}) {
  const content = (
    <>
      <div className="flex items-center justify-between">
        {presentation === "drawer" ? (
          <DrawerTitle className="text-base">Tasks</DrawerTitle>
        ) : (
          <SheetTitle className="text-base">Tasks</SheetTitle>
        )}
        <div className="flex items-center gap-2">
          {workspaceId && (
            <Button
              size="sm"
              variant="outline"
              className="h-7 gap-1 cursor-pointer"
              onClick={onQuickChat}
              data-testid="mobile-sheet-quick-chat"
            >
              <IconMessageCircle className="h-4 w-4" />
              Chat
            </Button>
          )}
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1 cursor-pointer"
            onClick={onNewTask}
          >
            <IconPlus className="h-4 w-4" />
            New
          </Button>
        </div>
      </div>
      <div className="pt-2">
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeWorkspaceId={workspaceId}
          onSelect={onWorkspaceChange}
        />
      </div>
    </>
  );
  if (presentation === "drawer") {
    return (
      <DrawerHeader className="shrink-0 border-b border-border p-4 pb-2 text-left">
        {content}
      </DrawerHeader>
    );
  }
  return <SheetHeader className="shrink-0 border-b border-border p-4 pb-2">{content}</SheetHeader>;
}

function surfaceAction<TArgs extends unknown[]>(
  presentation: "sheet" | "drawer",
  onOpenChange: (open: boolean) => void,
  action: (...args: TArgs) => unknown,
): (...args: TArgs) => void;
function surfaceAction<TArgs extends unknown[]>(
  presentation: "sheet" | "drawer",
  onOpenChange: (open: boolean) => void,
  action: ((...args: TArgs) => unknown) | undefined,
): ((...args: TArgs) => void) | undefined;
function surfaceAction<TArgs extends unknown[]>(
  presentation: "sheet" | "drawer",
  onOpenChange: (open: boolean) => void,
  action: ((...args: TArgs) => unknown) | undefined,
): ((...args: TArgs) => void) | undefined {
  if (!action || presentation === "sheet") return action;
  return (...args) => {
    onOpenChange(false);
    action(...args);
  };
}

type TaskSwitcherSurfaceContentProps = {
  presentation: "sheet" | "drawer";
  workspaceId: string | null;
  onOpenChange: (open: boolean) => void;
  onQuickChat: () => void;
  onNewTask: () => void;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  linking: ReturnType<typeof useMobileTaskLinking>;
};

function TaskSwitcherSurfaceContent({
  presentation,
  workspaceId,
  onOpenChange,
  onQuickChat,
  onNewTask,
  data,
  actions,
  rename,
  linking,
}: TaskSwitcherSurfaceContentProps) {
  return (
    <>
      <TaskSwitcherSurfaceHeader
        presentation={presentation}
        workspaceId={workspaceId}
        workspaces={data.workspaces.map((w) => ({ id: w.id, name: w.name }))}
        onWorkspaceChange={actions.handleWorkspaceChange}
        onQuickChat={onQuickChat}
        onNewTask={onNewTask}
      />
      <div className="shrink-0">
        <SidebarFilterBar />
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto p-2">
        <MobileTaskList
          tasks={data.tasksWithRepositories}
          workflows={data.workflows}
          stepsByWorkflowId={data.stepsByWorkflowId}
          activeTaskId={data.activeTaskId}
          selectedTaskId={data.selectedTaskId}
          onSelectTask={actions.handleSelectTask}
          onRenameTask={surfaceAction(presentation, onOpenChange, rename.handleRenameTask)}
          onArchiveTask={surfaceAction(presentation, onOpenChange, actions.handleArchiveTask)}
          onDeleteTask={surfaceAction(presentation, onOpenChange, actions.handleDeleteTask)}
          onDetachTask={surfaceAction(presentation, onOpenChange, actions.handleDetachTask)}
          onLinkPullRequest={surfaceAction(
            presentation,
            onOpenChange,
            linking.taskListHandlers.onLinkPullRequest,
          )}
          onLinkIssue={surfaceAction(
            presentation,
            onOpenChange,
            linking.taskListHandlers.onLinkIssue,
          )}
          onLinkJiraTicket={surfaceAction(
            presentation,
            onOpenChange,
            linking.taskListHandlers.onLinkJiraTicket,
          )}
          onLinkLinearIssue={surfaceAction(
            presentation,
            onOpenChange,
            linking.taskListHandlers.onLinkLinearIssue,
          )}
          onLinkSentryIssue={surfaceAction(
            presentation,
            onOpenChange,
            linking.taskListHandlers.onLinkSentryIssue,
          )}
          deletingTaskId={actions.deletingTaskId}
          isLoading={data.tasksLoading}
        />
      </div>
    </>
  );
}

function TaskSwitcherDialogs({
  dialogOpen,
  onDialogOpenChange,
  workspaceId,
  workflowId,
  data,
  actions,
  rename,
  linking,
}: {
  dialogOpen: boolean;
  onDialogOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  linking: ReturnType<typeof useMobileTaskLinking>;
}) {
  return (
    <>
      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={onDialogOpenChange}
        mode="create"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={data.dialogSteps[0]?.id ?? null}
        steps={data.dialogSteps}
        onSuccess={actions.handleTaskCreated}
      />
      <TaskArchiveConfirmDialog
        open={actions.archivingTask !== null}
        onOpenChange={(open) => {
          if (!open) actions.setArchivingTask(null);
        }}
        taskTitle={actions.archivingTask?.title ?? ""}
        taskId={actions.archivingTask?.id}
        executorType={actions.archivingTask?.executorType}
        isArchiving={actions.isArchiving}
        onConfirm={({ cascade }) => actions.handleArchiveConfirm({ cascade })}
      />
      <TaskRenameDialog
        open={rename.renamingTask !== null}
        onOpenChange={(open) => {
          if (!open) rename.setRenamingTask(null);
        }}
        currentTitle={rename.renamingTask?.title ?? ""}
        onSubmit={rename.handleRenameSubmit}
      />
      <TaskDeleteConfirmDialog
        open={actions.deletingTask !== null}
        onOpenChange={(open) => {
          if (!open) actions.setDeletingTask(null);
        }}
        taskTitle={actions.deletingTask?.title ?? ""}
        taskId={actions.deletingTask?.id}
        executorType={actions.deletingTask?.executorType}
        isDeleting={actions.isDeleting}
        onConfirm={({ cascade }) => actions.handleDeleteConfirm({ cascade })}
      />
      <TaskDetachTargetConfirmDialog
        target={actions.detachingTask}
        detachingTaskId={actions.detachingTaskId}
        onDismiss={() => actions.setDetachingTask(null)}
        onConfirm={actions.handleDetachConfirm}
      />
      <SidebarLinkDialogs
        actions={linking.actions}
        repositories={linking.repositories}
        workspaceId={workspaceId}
      />
    </>
  );
}

export const SessionTaskSwitcherSheet = memo(function SessionTaskSwitcherSheet({
  open,
  onOpenChange,
  workspaceId,
  workflowId,
  presentation = "sheet",
}: SessionTaskSwitcherSheetProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const data = useSheetData(workspaceId);
  const actions = useSheetActions(workspaceId, onOpenChange);
  const rename = useMobileTaskRename();
  const linking = useMobileTaskLinking(workspaceId, data.linkTaskById);
  const openQuickChat = useQuickChatLauncher(workspaceId);
  const handleQuickChat = useCallback(() => {
    onOpenChange(false);
    openQuickChat();
  }, [onOpenChange, openQuickChat]);

  const surfaceContent = (
    <TaskSwitcherSurfaceContent
      presentation={presentation}
      workspaceId={workspaceId}
      onOpenChange={onOpenChange}
      onQuickChat={handleQuickChat}
      onNewTask={() => {
        if (presentation === "drawer") onOpenChange(false);
        setDialogOpen(true);
      }}
      data={data}
      actions={actions}
      rename={rename}
      linking={linking}
    />
  );

  const surface =
    presentation === "drawer" ? (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="h-[88dvh] max-h-[88dvh] overflow-hidden pb-[max(0.5rem,env(safe-area-inset-bottom))]">
          {surfaceContent}
        </DrawerContent>
      </Drawer>
    ) : (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          showCloseButton={false}
          side="left"
          className="w-[85vw] max-w-sm p-0 flex flex-col"
        >
          {surfaceContent}
        </SheetContent>
      </Sheet>
    );

  return (
    <>
      {surface}
      <TaskSwitcherDialogs
        dialogOpen={dialogOpen}
        onDialogOpenChange={setDialogOpen}
        workspaceId={workspaceId}
        workflowId={workflowId}
        data={data}
        actions={actions}
        rename={rename}
        linking={linking}
      />
    </>
  );
});
