"use client";

import { useCallback, useRef, useState, memo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconNetwork, IconPlus } from "@tabler/icons-react";
import { SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useTaskSheetSelectionController } from "./task-sheet-selection-context";
export { useTaskSheetSelectionController } from "./task-sheet-selection-context";
import type { TaskSheetSelectionController } from "./session-task-switcher-sheet-selection";
import { useTaskReadStatus } from "./session-task-switcher-sheet-read-status";
import { TaskPickerSurface, InlineTaskHeader } from "./task-picker-surface";
import { Button } from "@kandev/ui/button";
import { QuickChatSheetButton } from "./quick-chat-sheet-button";
import { MobileTaskMoveOptionsSurface, useMobileTaskMoveOptions } from "./mobile-task-move-options";
import { SidebarFilterBar } from "../sidebar-filter/sidebar-filter-bar";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { WorkspaceSwitcher } from "../workspace-switcher";
import {
  PluginTaskLinkActionSurfaceProvider,
  useSidebarLinkActions,
} from "../task-session-sidebar-link-actions";
import { useSidebarTaskLinking } from "../task-session-sidebar-task-linking";
import { useSheetData, useSheetActions } from "./session-task-switcher-sheet-hooks";
import { handleTaskSheetOpenChange } from "./session-task-switcher-sheet-selection";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useMobileTaskRename } from "./use-mobile-task-rename";
import { useSidebarTaskEdit } from "../task-session-sidebar-edit";
import { useOptionalPortForwardingVisibility } from "../port-forwarding-visibility-provider";
import { TaskSwitcherDialogs } from "./session-task-switcher-sheet-dialogs";
import { MobileTaskList, type MobileTaskListProps } from "./mobile-task-list";
export { MobileTaskList, type MobileTaskListProps } from "./mobile-task-list";
type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  presentation?: "sheet" | "drawer";
  navigate?: (taskId: string, sessionId?: string) => void;
  onCloseAutoFocus?: (event: Event) => void;
  renderInline?: (body: ReactNode) => ReactNode;
  selection?: TaskSheetSelectionController;
};
function useTaskSheetOpener(open: boolean) {
  const [opener, setOpener] = useState({ open: false, current: null as HTMLElement | null });
  if (opener.open !== open) {
    setOpener({
      open,
      current:
        open && document.activeElement instanceof HTMLElement
          ? document.activeElement
          : opener.current,
    });
  }
  return opener;
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
          <Button variant="outline" className="gap-1 cursor-pointer" onClick={onNewTask}>
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
  inline?: boolean;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  onCreateSubtask: (taskId: string, taskTitle: string) => void;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  edit: ReturnType<typeof useSidebarTaskEdit>;
  linking: ReturnType<typeof useMobileTaskLinking>;
};

function mobileMoveActions(
  presentation: "sheet" | "drawer",
  moveOptions: ReturnType<typeof useMobileTaskMoveOptions>,
  onOpenChange: (open: boolean) => void,
) {
  if (presentation !== "drawer") return {};
  return {
    onMoveToStep: moveOptions.handleMove,
    onRequestMoveOptions: moveOptions.handleRequest,
    onBeforeMoveOptionsOpen: () => onOpenChange(false),
  };
}

// eslint-disable-next-line max-lines-per-function -- this surface keeps the existing mobile scroll owner intact
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
  inline = false,
  expanded,
  onExpandedChange,
}: TaskSwitcherSurfaceContentProps) {
  const { t } = useTranslation();
  const listScrollRef = useRef<HTMLDivElement | null>(null);
  const { taskLoadError, retryTaskLoad } = useTaskReadStatus(data);
  const moveOptions = useMobileTaskMoveOptions({
    open,
    activeTaskId: data.activeTaskId,
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
      {inline ? (
        <InlineTaskHeader
          expanded={expanded}
          onExpandedChange={onExpandedChange}
          onNewTask={onNewTask}
        />
      ) : (
        <TaskSwitcherSurfaceHeader
          presentation={presentation}
          workspaceId={workspaceId}
          workspaces={data.workspaces.map((w) => ({ id: w.id, name: w.name }))}
          onWorkspaceChange={actions.handleWorkspaceChange}
          onQuickChat={onQuickChat}
          onNewTask={onNewTask}
        />
      )}
      <div
        id={inline ? "mobile-navigation-task-body" : undefined}
        hidden={inline && !expanded}
        className={inline ? undefined : "flex min-h-0 flex-1 flex-col"}
      >
        {data.activeTaskId && (
          <PortForwardingTaskAction
            onClose={() => onOpenChange(false)}
            activeTaskId={data.activeTaskId}
          />
        )}
        <div className="shrink-0">
          <SidebarFilterBar />
        </div>
        <div
          className={inline ? "p-2" : "flex-1 min-h-0 overflow-y-auto p-2"}
          data-testid="mobile-task-switcher-list"
          ref={listScrollRef}
        >
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
              {...mobileMoveActions(presentation, moveOptions, onOpenChange)}
              {...taskListActions}
              onCreateSubtask={onCreateSubtask}
              onNestTask={actions.handleNestTask}
              deletingTaskId={actions.deletingTaskId}
              archivingTaskId={actions.archivingTaskId}
              isArchiving={actions.isArchiving}
              isLoading={
                data.tasksLoading ||
                (data.workspaceContextPending && data.tasksWithRepositories.length === 0)
              }
              loadError={taskLoadError}
              onRetryLoad={retryTaskLoad}
              retryLabel={t("sidebar:retry")}
              pageEntries={data.pageEntries}
              page={data.page.response}
              pagePending={data.page.requestedPage !== null}
              pageError={data.page.error}
              pageCanRetry={data.page.canRetry}
              onPageChange={data.page.goToPage}
              onPageRetry={data.page.retry}
              scrollContainerRef={listScrollRef}
            />
          </PluginTaskLinkActionSurfaceProvider>
        </div>
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
  const visibility = useOptionalPortForwardingVisibility();
  if (!visibility) return null;
  const { enabled, canToggle, isUpdating, togglePortForwarding } = visibility;

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
  navigate,
  onCloseAutoFocus,
  renderInline,
  selection,
}: SessionTaskSwitcherSheetProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [expanded, setExpanded] = useState(true);
  const opener = useTaskSheetOpener(open);
  const autoFocusNewTasks = useAppStore((state) => state.userSettings.autoFocusNewTasks) !== false;
  const [subtaskTarget, setSubtaskTarget] = useState<{ id: string; title: string } | null>(null);
  const data = useSheetData(workspaceId);
  const localSelection = useTaskSheetSelectionController();
  const selectionController = selection ?? localSelection;
  const handleOpenChange = useCallback(
    (nextOpen: boolean) => handleTaskSheetOpenChange(selectionController, nextOpen, onOpenChange),
    [onOpenChange, selectionController],
  );
  const actions = useSheetActions(workspaceId, handleOpenChange, selectionController, navigate);
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
      inline={!!renderInline}
      expanded={expanded}
      onExpandedChange={setExpanded}
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

  const surface = renderInline ? (
    renderInline(surfaceContent)
  ) : (
    <TaskPickerSurface
      presentation={presentation}
      workspaceId={workspaceId}
      open={open}
      onOpenChange={handleOpenChange}
      onCloseAutoFocus={onCloseAutoFocus}
      restoreFocusOnClose={!dialogOpen}
    >
      {surfaceContent}
    </TaskPickerSurface>
  );

  return (
    <>
      {surface}
      <TaskSwitcherDialogs
        focusReturnRef={presentation === "drawer" && !autoFocusNewTasks ? opener : undefined}
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
