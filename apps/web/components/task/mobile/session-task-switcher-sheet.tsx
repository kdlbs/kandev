"use client";

import { useCallback, useEffect, useMemo, useState, memo } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconNetwork, IconPlus } from "@tabler/icons-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { QuickChatSheetButton } from "./quick-chat-sheet-button";
import { TaskSwitcher } from "../task-switcher";
import type { TaskSwitcherItem } from "../task-switcher";
import type { TaskMoveWorkflow } from "../task-move-context-menu";
import { MobileTaskMoveOptionsSurface, useMobileTaskMoveOptions } from "./mobile-task-move-options";
import { SidebarFilterBar } from "../sidebar-filter/sidebar-filter-bar";
import type { StepDef } from "../task-switcher-context-menu";
import { applyView } from "@/lib/sidebar/apply-view";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { WorkspaceSwitcher } from "../workspace-switcher";
import {
  PluginTaskLinkActionSurfaceProvider,
  useSidebarLinkActions,
} from "../task-session-sidebar-link-actions";
import { useSidebarTaskLinking } from "../task-session-sidebar-task-linking";
import { useSheetData, useSheetActions } from "./session-task-switcher-sheet-hooks";
import {
  createTaskSheetSelectionController,
  handleTaskSheetOpenChange,
} from "./session-task-switcher-sheet-selection";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useMobileTaskRename } from "./use-mobile-task-rename";
import { useSidebarTaskEdit } from "../task-session-sidebar-edit";
import { usePortForwardingVisibility } from "../port-forwarding-visibility-provider";
import { buildMobileTaskSwitcherProps } from "./session-task-switcher-sheet-props";
import { TaskSwitcherDialogs } from "./session-task-switcher-sheet-dialogs";
type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  presentation?: "sheet" | "drawer";
};
export function useTaskSheetSelectionController() {
  const [selectionController] = useState(createTaskSheetSelectionController);
  useEffect(
    () => () => {
      selectionController.invalidate();
    },
    [selectionController],
  );
  return selectionController;
}
export function useMobileTaskLinking(workspaceId: string | null) {
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

export type MobileTaskListProps = {
  tasks: TaskSwitcherItem[];
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRequestMoveOptions?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onBeforeMoveOptionsOpen?: () => void;
  onEditTask?: (task: TaskSwitcherItem) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
  onArchiveTask: (taskId: string, opts?: { cascade?: boolean }) => void;
  onDeleteTask: (taskId: string) => Promise<void> | void;
  onDetachTask: (taskId: string) => Promise<void> | void;
  archivingTaskId?: string | null;
  isArchiving?: boolean;
  onNestTask?: (taskId: string, parentTaskId: string) => void;
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
};

/**
 * The mobile task tree surface: renders the shared TaskSwitcher with the
 * mobile drawer's view state (grouping, ordering, collapse, reorder, nest).
 */
export function MobileTaskList(props: MobileTaskListProps) {
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
      applyView(props.tasks, view, {
        pinnedTaskIds,
        orderedTaskIds,
        subtaskOrderByParentId,
      }),
    [props.tasks, view, pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId, i18n.language],
  );
  const switcherProps = buildMobileTaskSwitcherProps(props, {
    grouped,
    collapsedGroupKeys: view.collapsedGroups,
    onToggleGroup: handleToggleGroup,
    collapsedSubtaskParentIds: collapsedSubtaskParents,
    onToggleSubtasks: toggleSubtaskCollapsed,
    onTogglePin: togglePinnedTask,
    onReorderGroup: handleReorderGroup,
    onReorderSubtasks: handleReorderSubtasks,
    pinnedTaskIds,
    showActivityTime: view.sort.key === "lastActivityAt",
    taskRowPresentation: view.taskRow,
  });
  return <TaskSwitcher {...switcherProps} />;
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
          {workspaceId && <QuickChatSheetButton workspaceId={workspaceId} onClick={onQuickChat} />}
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

function buildMobileTaskListSurfaceActions({
  presentation,
  onOpenChange,
  actions,
  rename,
  edit,
  linking,
}: Pick<
  TaskSwitcherSurfaceContentProps,
  "presentation" | "onOpenChange" | "actions" | "rename" | "edit" | "linking"
>): Pick<
  MobileTaskListProps,
  | "onEditTask"
  | "onRenameTask"
  | "onArchiveTask"
  | "onDeleteTask"
  | "onDetachTask"
  | "onLinkPullRequest"
  | "onLinkIssue"
  | "onLinkMergeRequest"
  | "onLinkJiraTicket"
  | "onLinkLinearIssue"
  | "onLinkSentryIssue"
> {
  return {
    onEditTask: surfaceAction(presentation, onOpenChange, edit.handleEditTask),
    onRenameTask: surfaceAction(presentation, onOpenChange, rename.handleRenameTask),
    onArchiveTask: surfaceAction(presentation, onOpenChange, actions.handleArchiveTask),
    onDeleteTask: surfaceAction(presentation, onOpenChange, actions.handleDeleteTask),
    onDetachTask: surfaceAction(presentation, onOpenChange, actions.handleDetachTask),
    onLinkPullRequest: surfaceAction(
      presentation,
      onOpenChange,
      linking.taskListHandlers.onLinkPullRequest,
    ),
    onLinkIssue: surfaceAction(presentation, onOpenChange, linking.taskListHandlers.onLinkIssue),
    onLinkMergeRequest: surfaceAction(
      presentation,
      onOpenChange,
      linking.taskListHandlers.onLinkMergeRequest,
    ),
    onLinkJiraTicket: surfaceAction(
      presentation,
      onOpenChange,
      linking.taskListHandlers.onLinkJiraTicket,
    ),
    onLinkLinearIssue: surfaceAction(
      presentation,
      onOpenChange,
      linking.taskListHandlers.onLinkLinearIssue,
    ),
    onLinkSentryIssue: surfaceAction(
      presentation,
      onOpenChange,
      linking.taskListHandlers.onLinkSentryIssue,
    ),
  };
}

type TaskSwitcherSurfaceContentProps = {
  open: boolean;
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

function TaskSwitcherSurfaceContent({
  open,
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
  const moveOptions = useMobileTaskMoveOptions({
    open,
    stepsByWorkflowId: data.stepsByWorkflowId,
  });
  if (moveOptions.moveOptionsStep && moveOptions.request) {
    return (
      <MobileTaskMoveOptionsSurface
        presentation={presentation}
        step={moveOptions.moveOptionsStep}
        isMoving={moveOptions.isMoving}
        onBack={moveOptions.handleClose}
        onSubmit={moveOptions.handleSubmit}
      />
    );
  }
  const taskListActions = buildMobileTaskListSurfaceActions({
    presentation,
    onOpenChange,
    actions,
    rename,
    edit,
    linking,
  });
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
      {data.activeTaskId && (
        <PortForwardingTaskAction
          onClose={() => onOpenChange(false)}
          activeTaskId={data.activeTaskId}
        />
      )}
      <div className="shrink-0">
        <SidebarFilterBar />
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto p-2" data-testid="mobile-task-switcher-list">
        <PluginTaskLinkActionSurfaceProvider
          beforePluginRun={presentation === "drawer" ? () => onOpenChange(false) : undefined}
        >
          <MobileTaskList
            tasks={data.tasksWithRepositories}
            workflows={data.workflows}
            stepsByWorkflowId={data.stepsByWorkflowId}
            activeTaskId={data.activeTaskId}
            selectedTaskId={data.selectedTaskId}
            onSelectTask={actions.handleSelectTask}
            onRequestMoveOptions={presentation === "drawer" ? moveOptions.handleRequest : undefined}
            onBeforeMoveOptionsOpen={
              presentation === "drawer" ? () => onOpenChange(false) : undefined
            }
            {...taskListActions}
            onCreateSubtask={onCreateSubtask}
            onNestTask={actions.handleNestTask}
            deletingTaskId={actions.deletingTaskId}
            archivingTaskId={actions.archivingTaskId}
            isArchiving={actions.isArchiving}
            isLoading={data.tasksLoading}
            loadError={data.archivedError ? t("sidebar:archivedLoadFailed") : null}
            onRetryLoad={data.retryArchivedTasks}
            retryLabel={t("sidebar:retry")}
          />
        </PluginTaskLinkActionSurfaceProvider>
      </div>
    </>
  );
}

function PortForwardingTaskAction({
  activeTaskId,
  onClose,
}: {
  activeTaskId: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { enabled, canToggle, isUpdating, togglePortForwarding } = usePortForwardingVisibility();

  return (
    <div className="shrink-0 border-b border-border px-2 py-1">
      <Button
        variant="ghost"
        className="min-h-11 w-full justify-start gap-3 px-3 text-sm"
        data-testid="mobile-port-forwarding-toggle"
        aria-pressed={enabled}
        disabled={!canToggle || isUpdating || !activeTaskId}
        onClick={() => {
          onClose();
          void togglePortForwarding({ openDialogOnEnable: true });
        }}
      >
        <IconNetwork className="h-4 w-4 shrink-0" />
        <span className="min-w-0 flex-1 text-left">{t("task:portForwarding")}</span>
        {enabled && <IconCheck className="h-4 w-4 shrink-0" />}
      </Button>
    </div>
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
  const selectionController = useTaskSheetSelectionController();
  const handleOpenChange = useCallback(
    (nextOpen: boolean) => handleTaskSheetOpenChange(selectionController, nextOpen, onOpenChange),
    [onOpenChange, selectionController],
  );
  const actions = useSheetActions(workspaceId, handleOpenChange, selectionController);
  const rename = useMobileTaskRename();
  const edit = useSidebarTaskEdit();
  const linking = useMobileTaskLinking(workspaceId);
  const openQuickChat = useQuickChatLauncher(workspaceId);
  const handleQuickChat = useCallback(() => {
    handleOpenChange(false);
    openQuickChat();
  }, [handleOpenChange, openQuickChat]);
  const handleCreateSubtask = useCallback(
    (taskId: string, taskTitle: string) => {
      handleOpenChange(false);
      setSubtaskTarget({ id: taskId, title: taskTitle });
    },
    [handleOpenChange],
  );

  const surfaceContent = (
    <TaskSwitcherSurfaceContent
      open={open}
      presentation={presentation}
      workspaceId={workspaceId}
      onOpenChange={handleOpenChange}
      onQuickChat={handleQuickChat}
      onCreateSubtask={handleCreateSubtask}
      onNewTask={() => {
        if (presentation === "drawer") handleOpenChange(false);
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
      <Drawer open={open} onOpenChange={handleOpenChange}>
        <DrawerContent className="h-[88dvh] max-h-[88dvh] overflow-hidden pb-[max(0.5rem,env(safe-area-inset-bottom))]">
          {surfaceContent}
        </DrawerContent>
      </Drawer>
    ) : (
      <Sheet open={open} onOpenChange={handleOpenChange}>
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
