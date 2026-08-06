"use client";

import { useCallback, useMemo, useState, memo } from "react";
import { useTranslation } from "react-i18next";
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
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { WorkspaceSwitcher } from "../workspace-switcher";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { NewSubtaskDialog } from "../new-subtask-dialog";
import { TaskArchiveConfirmDialog } from "../task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "../task-delete-confirm-dialog";
import { TaskDetachTargetConfirmDialog } from "../task-detach-confirm-dialog";
import { TaskRenameDialog } from "../task-rename-dialog";
import { SidebarLinkDialogs } from "../task-session-sidebar-dialogs";
import { useSidebarLinkActions } from "../task-session-sidebar-link-actions";
import { useSidebarTaskLinking } from "../task-session-sidebar-task-linking";
import { useSheetData, useSheetActions } from "./session-task-switcher-sheet-hooks";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useMobileTaskRename } from "./use-mobile-task-rename";
import { SidebarTaskEditDialog, useSidebarTaskEdit } from "../task-session-sidebar-edit";

type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  presentation?: "sheet" | "drawer";
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

function useSidebarGroupToggle(viewId: string) {
  const toggleSidebarGroupCollapsed = useAppStore((s) => s.toggleSidebarGroupCollapsed);
  return useCallback(
    (groupKey: string) => toggleSidebarGroupCollapsed(viewId, groupKey),
    [toggleSidebarGroupCollapsed, viewId],
  );
}

// eslint-disable-next-line max-lines-per-function -- assembles the shared task tree with the mobile drawer's view state.
export function MobileTaskList({
  tasks,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onEditTask,
  onRenameTask,
  onCreateSubtask,
  onArchiveTask,
  onDeleteTask,
  onDetachTask,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  deletingTaskId,
  isLoading,
  loadError,
  onRetryLoad,
  retryLabel,
}: {
  tasks: TaskSwitcherItem[];
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onEditTask?: (task: TaskSwitcherItem) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
  onArchiveTask: (taskId: string) => void;
  onDeleteTask: (taskId: string) => Promise<void> | void;
  onDetachTask: (taskId: string) => Promise<void> | void;
  onLinkPullRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkMergeRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkJiraTicket?: (taskId: string, taskTitle?: string) => void;
  onLinkLinearIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkSentryIssue?: (taskId: string, taskTitle?: string) => void;
  deletingTaskId: string | null;
  isLoading?: boolean;
  loadError?: string | null;
  onRetryLoad?: () => void;
  retryLabel?: string;
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
  const collapsedSubtaskParents = useAppStore((s) => s.collapsedSubtaskParents);
  const toggleSubtaskCollapsed = useAppStore((s) => s.toggleSubtaskCollapsed);
  const handleToggleGroup = useSidebarGroupToggle(view.id);
  // See useGroupedSidebarView: the executorType group label is catalog-backed.
  const { i18n } = useTranslation();
  const grouped = useMemo(
    () =>
      applyView(tasks, view, {
        pinnedTaskIds,
        orderedTaskIds,
        subtaskOrderByParentId,
      }),
    [tasks, view, pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId, i18n.language],
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
      onEditTask={onEditTask}
      onRenameTask={onRenameTask}
      onCreateSubtask={onCreateSubtask}
      onArchiveTask={onArchiveTask}
      onDeleteTask={onDeleteTask}
      onDetachTask={onDetachTask}
      onLinkPullRequest={onLinkPullRequest}
      onLinkIssue={onLinkIssue}
      onLinkMergeRequest={onLinkMergeRequest}
      onLinkJiraTicket={onLinkJiraTicket}
      onLinkLinearIssue={onLinkLinearIssue}
      onLinkSentryIssue={onLinkSentryIssue}
      onTogglePin={togglePinnedTask}
      onReorderGroup={handleReorderGroup}
      onReorderSubtasks={handleReorderSubtasks}
      pinnedTaskIds={pinnedTaskIds}
      deletingTaskId={deletingTaskId}
      isLoading={isLoading}
      loadError={loadError}
      onRetryLoad={onRetryLoad}
      retryLabel={retryLabel}
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
  const { t } = useTranslation();
  const content = (
    <>
      <div className="flex items-center justify-between">
        {presentation === "drawer" ? (
          <DrawerTitle className="text-base">{t("task:tasks")}</DrawerTitle>
        ) : (
          <SheetTitle className="text-base">{t("task:tasks")}</SheetTitle>
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
              {t("task:chat")}
            </Button>
          )}
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1 cursor-pointer"
            onClick={onNewTask}
          >
            <IconPlus className="h-4 w-4" />
            {t("task:new")}
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
  onCreateSubtask: (taskId: string, taskTitle: string) => void;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  edit: ReturnType<typeof useSidebarTaskEdit>;
  linking: ReturnType<typeof useMobileTaskLinking>;
};

function MobileSubtaskDialog({
  target,
  onTargetChange,
}: {
  target: { id: string; title: string } | null;
  onTargetChange: (next: { id: string; title: string } | null) => void;
}) {
  return (
    <NewSubtaskDialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onTargetChange(null);
      }}
      parentTaskId={target?.id ?? ""}
      parentTaskTitle={target?.title ?? ""}
    />
  );
}

function TaskSwitcherSurfaceContent({
  presentation,
  workspaceId,
  onOpenChange,
  onQuickChat,
  onNewTask,
  onCreateSubtask,
  data,
  actions,
  rename,
  edit,
  linking,
}: TaskSwitcherSurfaceContentProps) {
  const { t } = useTranslation();
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
      <div className="flex-1 min-h-0 overflow-y-auto p-2" data-testid="mobile-task-switcher-list">
        <MobileTaskList
          tasks={data.tasksWithRepositories}
          workflows={data.workflows}
          stepsByWorkflowId={data.stepsByWorkflowId}
          activeTaskId={data.activeTaskId}
          selectedTaskId={data.selectedTaskId}
          onSelectTask={actions.handleSelectTask}
          onEditTask={surfaceAction(presentation, onOpenChange, edit.handleEditTask)}
          onRenameTask={surfaceAction(presentation, onOpenChange, rename.handleRenameTask)}
          onCreateSubtask={onCreateSubtask}
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
          onLinkMergeRequest={surfaceAction(
            presentation,
            onOpenChange,
            linking.taskListHandlers.onLinkMergeRequest,
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
          loadError={data.archivedError ? t("sidebar:archivedLoadFailed") : null}
          onRetryLoad={data.retryArchivedTasks}
          retryLabel={t("sidebar:retry")}
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
  edit,
  linking,
  subtaskTarget,
  onSubtaskTargetChange,
}: {
  dialogOpen: boolean;
  onDialogOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  edit: ReturnType<typeof useSidebarTaskEdit>;
  linking: ReturnType<typeof useMobileTaskLinking>;
  subtaskTarget: { id: string; title: string } | null;
  onSubtaskTargetChange: (target: { id: string; title: string } | null) => void;
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
      <MobileSubtaskDialog target={subtaskTarget} onTargetChange={onSubtaskTargetChange} />
      <SidebarTaskEditDialog
        target={edit.editingTask}
        onTargetChange={edit.setEditingTask}
        workspaceId={workspaceId}
        stepsByWorkflowId={data.stepsByWorkflowId}
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
  const [subtaskTarget, setSubtaskTarget] = useState<{ id: string; title: string } | null>(null);
  const data = useSheetData(workspaceId);
  const actions = useSheetActions(workspaceId, onOpenChange);
  const rename = useMobileTaskRename();
  const edit = useSidebarTaskEdit();
  const linking = useMobileTaskLinking(workspaceId);
  const openQuickChat = useQuickChatLauncher(workspaceId);
  const handleQuickChat = useCallback(() => {
    onOpenChange(false);
    openQuickChat();
  }, [onOpenChange, openQuickChat]);
  const handleCreateSubtask = useCallback(
    (taskId: string, taskTitle: string) => {
      onOpenChange(false);
      setSubtaskTarget({ id: taskId, title: taskTitle });
    },
    [onOpenChange],
  );

  const surfaceContent = (
    <TaskSwitcherSurfaceContent
      presentation={presentation}
      workspaceId={workspaceId}
      onOpenChange={onOpenChange}
      onQuickChat={handleQuickChat}
      onCreateSubtask={handleCreateSubtask}
      onNewTask={() => {
        if (presentation === "drawer") onOpenChange(false);
        setDialogOpen(true);
      }}
      data={data}
      actions={actions}
      rename={rename}
      edit={edit}
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
        edit={edit}
        linking={linking}
        subtaskTarget={subtaskTarget}
        onSubtaskTargetChange={setSubtaskTarget}
      />
    </>
  );
});
