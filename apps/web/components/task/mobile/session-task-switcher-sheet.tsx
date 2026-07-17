"use client";

import { useCallback, useMemo, useState, memo } from "react";
import { IconMessageCircle, IconPlus } from "@tabler/icons-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Button } from "@kandev/ui/button";
import { TaskSwitcher } from "../task-switcher";
import type { TaskSwitcherItem } from "../task-switcher";
import { SidebarFilterBar } from "../sidebar-filter/sidebar-filter-bar";
import type { StepDef } from "../task-switcher-context-menu";
import type { TaskMoveWorkflow } from "../task-move-context-menu";
import { applyView } from "@/lib/sidebar/apply-view";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { WorkspaceSwitcher } from "../workspace-switcher";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { TaskArchiveConfirmDialog } from "../task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "../task-delete-confirm-dialog";
import { TaskRenameDialog } from "../task-rename-dialog";
import { SidebarLinkDialogs } from "../task-session-sidebar-dialogs";
import { useSidebarLinkActions } from "../task-session-sidebar-link-actions";
import { useSidebarTaskLinking } from "../task-session-sidebar-task-linking";
import { useSheetData, useSheetActions } from "./session-task-switcher-sheet-hooks";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useMobileTaskRename } from "./use-mobile-task-rename";

type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
};

function useMobileTaskLinking(workspaceId: string | null) {
  const store = useAppStoreApi();
  const actions = useSidebarLinkActions(store);
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

function TaskSwitcherSheetHeader({
  workspaceId,
  workspaces,
  onWorkspaceChange,
  onQuickChat,
  onNewTask,
}: {
  workspaceId: string | null;
  workspaces: Array<{ id: string; name: string }>;
  onWorkspaceChange: (workspaceId: string) => void;
  onQuickChat: () => void;
  onNewTask: () => void;
}) {
  return (
    <SheetHeader className="p-4 pb-2 border-b border-border">
      <div className="flex items-center justify-between">
        <SheetTitle className="text-base">Tasks</SheetTitle>
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
    </SheetHeader>
  );
}

export const SessionTaskSwitcherSheet = memo(function SessionTaskSwitcherSheet({
  open,
  onOpenChange,
  workspaceId,
  workflowId,
}: SessionTaskSwitcherSheetProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const data = useSheetData(workspaceId);
  const actions = useSheetActions(workspaceId, onOpenChange);
  const rename = useMobileTaskRename();
  const linking = useMobileTaskLinking(workspaceId);
  const openQuickChat = useQuickChatLauncher(workspaceId);
  const handleQuickChat = useCallback(() => {
    onOpenChange(false);
    openQuickChat();
  }, [onOpenChange, openQuickChat]);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        showCloseButton={false}
        side="left"
        className="w-[85vw] max-w-sm p-0 flex flex-col"
      >
        <TaskSwitcherSheetHeader
          workspaceId={workspaceId}
          workspaces={data.workspaces.map((w) => ({ id: w.id, name: w.name }))}
          onWorkspaceChange={actions.handleWorkspaceChange}
          onQuickChat={handleQuickChat}
          onNewTask={() => setDialogOpen(true)}
        />

        <SidebarFilterBar />

        <div className="flex-1 min-h-0 overflow-y-auto p-2">
          <MobileTaskList
            tasks={data.tasksWithRepositories}
            workflows={data.workflows}
            stepsByWorkflowId={data.stepsByWorkflowId}
            activeTaskId={data.activeTaskId}
            selectedTaskId={data.selectedTaskId}
            onSelectTask={actions.handleSelectTask}
            onRenameTask={rename.handleRenameTask}
            onArchiveTask={actions.handleArchiveTask}
            onDeleteTask={actions.handleDeleteTask}
            {...linking.taskListHandlers}
            deletingTaskId={actions.deletingTaskId}
            isLoading={data.tasksLoading}
          />
        </div>
      </SheetContent>

      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
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
      <SidebarLinkDialogs
        actions={linking.actions}
        repositories={linking.repositories}
        workspaceId={workspaceId}
      />
    </Sheet>
  );
});
