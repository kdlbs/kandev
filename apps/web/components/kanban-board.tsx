"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { Task } from "./kanban-card";
import { TaskCreateDialog } from "./task-create-dialog";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { Task as BackendTask } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import { type MoveTaskError } from "@/hooks/use-drag-and-drop";
import { SwimlaneContainer } from "./kanban/swimlane-container";
import { KanbanHeader } from "./kanban/kanban-header";
import { MobileFab } from "./kanban/mobile-fab";
import { MobileSearchBar } from "./kanban/mobile-search-bar";
import { TaskMultiSelectToolbar } from "./kanban/task-multi-select-toolbar";
import { useKanbanData, useKanbanActions, useKanbanNavigation } from "@/hooks/domains/kanban";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { resolveBoardWorkflowId, resolveDesiredWorkflowId } from "@/lib/kanban/resolve-workflow";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskMultiSelect } from "@/hooks/use-task-multi-select";
import { HomepageCommands } from "./homepage-commands";
import { linkToTask } from "@/lib/links";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@kandev/ui/alert-dialog";
import { IconAlertTriangle } from "@tabler/icons-react";

function useWorkflowSelection({
  store,
  userSettings,
  workspaceState,
  workflowsState,
  commitSettings,
  setActiveWorkflow,
  setWorkflows,
}: {
  store: ReturnType<typeof useAppStoreApi>;
  userSettings: { workflowId?: string | null };
  workspaceState: { activeId: string | null };
  workflowsState: WorkflowsState;
  commitSettings: unknown;
  setActiveWorkflow: (id: string | null) => void;
  setWorkflows: (workflows: WorkflowsState["items"]) => void;
}) {
  const searchParams = useSearchParams();
  const routeWorkflowId = searchParams.get("workflowId");
  const userSettingsRef = useRef(userSettings);
  useEffect(() => {
    userSettingsRef.current = userSettings;
  });

  useEffect(() => {
    const workspaceId = workspaceState.activeId;
    if (!workspaceId) {
      if (workflowsState.items.length || workflowsState.activeId) {
        setWorkflows([]);
        setActiveWorkflow(null);
      }
      return;
    }
    const settings = userSettingsRef.current;
    const workspaceWorkflows = workflowsState.items.filter(
      (workflow: WorkflowsState["items"][number]) => workflow.workspaceId === workspaceId,
    );

    const desiredWorkflowId = resolveDesiredWorkflowId({
      activeWorkflowId: routeWorkflowId ?? workflowsState.activeId,
      settingsWorkflowId: settings.workflowId,
      workspaceWorkflows,
    });
    setActiveWorkflow(desiredWorkflowId);
    if (!desiredWorkflowId) {
      store.getState().hydrate({
        kanban: { workflowId: null, steps: [], tasks: [] },
      });
    }
  }, [
    workflowsState.activeId,
    workflowsState.items,
    commitSettings,
    setActiveWorkflow,
    setWorkflows,
    store,
    routeWorkflowId,
    workspaceState.activeId,
  ]);
}

function useMoveErrorState(router: ReturnType<typeof useRouter>) {
  const [moveError, setMoveError] = useState<MoveTaskError | null>(null);

  const handleMoveError = useCallback((error: MoveTaskError) => {
    setMoveError(error);
  }, []);

  const handleGoToTask = useCallback(() => {
    if (moveError?.taskId) {
      router.push(linkToTask(moveError.taskId));
    }
    setMoveError(null);
  }, [moveError, router]);

  return {
    moveError,
    setMoveError,
    handleMoveError,
    handleGoToTask,
  };
}

function useKanbanBoardStore() {
  const store = useAppStoreApi();
  const kanbanViewMode = useAppStore((state) => state.userSettings.kanbanViewMode);
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const workflowsState = useAppStore((state) => state.workflows);
  const setActiveWorkflow = useAppStore((state) => state.setActiveWorkflow);
  const setWorkflows = useAppStore((state) => state.setWorkflows);
  return {
    store,
    kanbanViewMode,
    kanban,
    workspaceState,
    workflowsState,
    setActiveWorkflow,
    setWorkflows,
  };
}

interface KanbanBoardProps {
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task) => void;
  /** Fired before the edit dialog opens so the preview panel can close itself. */
  onBeforeEdit?: () => void;
}

function useKanbanBoardHooks(
  searchQuery: string,
  workspaceState: ReturnType<typeof useKanbanBoardStore>["workspaceState"],
  workflowsState: ReturnType<typeof useKanbanBoardStore>["workflowsState"],
) {
  const {
    isDialogOpen,
    editingTask,
    setIsDialogOpen,
    setEditingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleArchive,
    handleDialogOpenChange,
    handleDialogSuccess,
    handleWorkspaceChange,
    handleWorkflowChange,
    deletingTaskId,
    archivingTaskId,
  } = useKanbanActions({ workspaceState, workflowsState });
  const { enablePreviewOnClick, userSettings, commitSettings, activeSteps, isMounted } =
    useKanbanData({
      onWorkspaceChange: handleWorkspaceChange,
      onWorkflowChange: handleWorkflowChange,
      searchQuery,
    });
  const handlePersistedWorkflowChange = useCallback(
    (workflowId: string | null) => {
      commitSettings({
        workspaceId: userSettings.workspaceId,
        workflowId,
        repositoryIds: userSettings.repositoryIds,
      });
      handleWorkflowChange(workflowId);
    },
    [commitSettings, handleWorkflowChange, userSettings.repositoryIds, userSettings.workspaceId],
  );
  return {
    isDialogOpen,
    editingTask,
    setIsDialogOpen,
    setEditingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleArchive,
    handleWorkflowChange: handlePersistedWorkflowChange,
    handleDialogOpenChange,
    handleDialogSuccess,
    deletingTaskId,
    archivingTaskId,
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    activeSteps,
    isMounted,
  };
}

type SnapEntry = {
  tasks: { id: string }[];
  steps: { id: string; title: string; color?: string | null }[];
};

function useMultiSelectDerived(
  selectedIds: Set<string>,
  snapshots: Record<string, SnapEntry>,
  activeSteps: { id: string; title: string; color?: string | null }[],
) {
  const isMixedWorkflowSelection = useMemo(() => {
    if (selectedIds.size === 0) return false;
    const taskToWorkflow = new Map<string, string>();
    for (const [wfId, snap] of Object.entries(snapshots)) {
      for (const task of snap.tasks) taskToWorkflow.set(task.id, wfId);
    }
    const wfIds = new Set<string>();
    for (const id of selectedIds) {
      const wfId = taskToWorkflow.get(id);
      if (wfId) wfIds.add(wfId);
    }
    return wfIds.size > 1;
  }, [selectedIds, snapshots]);

  const multiSelectSteps = useMemo(() => {
    if (selectedIds.size > 0) {
      for (const snap of Object.values(snapshots)) {
        if (snap.tasks.some((t) => selectedIds.has(t.id))) {
          return snap.steps.map((s) => ({ id: s.id, title: s.title, color: s.color ?? "" }));
        }
      }
    }
    return activeSteps.map((s) => ({ id: s.id, title: s.title, color: s.color ?? "" }));
  }, [selectedIds, snapshots, activeSteps]);

  return { isMixedWorkflowSelection, multiSelectSteps };
}

function useKanbanBoardSetup(
  onPreviewTask: KanbanBoardProps["onPreviewTask"],
  onOpenTask: KanbanBoardProps["onOpenTask"],
  onBeforeEdit: KanbanBoardProps["onBeforeEdit"],
) {
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();
  const [searchQuery, setSearchQuery] = useState("");
  const {
    store,
    kanbanViewMode,
    kanban,
    workspaceState,
    workflowsState,
    setActiveWorkflow,
    setWorkflows,
  } = useKanbanBoardStore();

  useAllWorkflowSnapshots(workspaceState.activeId);
  useWorkspacePRs(workspaceState.activeId);
  useWorkspaceMRs(workspaceState.activeId);

  const hooks = useKanbanBoardHooks(searchQuery, workspaceState, workflowsState);
  const { handleOpenTask, handleCardClick } = useKanbanNavigation({
    enablePreviewOnClick: hooks.enablePreviewOnClick,
    isMobile,
    onPreviewTask,
    onOpenTask,
  });
  // Close preview before the edit dialog opens; destructure for stable useCallback dep.
  const { handleEdit } = hooks;
  const handleEditWithCleanup = useCallback(
    (task: Task) => {
      onBeforeEdit?.();
      handleEdit(task);
    },
    [onBeforeEdit, handleEdit],
  );

  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const [mobileWorkflowFocusId, setMobileWorkflowFocusId] = useState<string | null>(null);
  const effectiveWorkflowId = resolveBoardWorkflowId({
    isMobile,
    selectedWorkflowId: workflowsState.activeId,
    focusedWorkflowId: mobileWorkflowFocusId,
    hydratedWorkflowId: kanban.workflowId,
  });
  const effectiveSteps = useMemo(() => {
    const focusedSnapshot = effectiveWorkflowId ? snapshots[effectiveWorkflowId] : undefined;
    if (!focusedSnapshot) return hooks.activeSteps;
    return [...focusedSnapshot.steps].sort((a, b) => a.position - b.position);
  }, [effectiveWorkflowId, hooks.activeSteps, snapshots]);

  const multiSelect = useTaskMultiSelect(effectiveWorkflowId);
  const { isMultiSelectMode, toggleSelect } = multiSelect;
  const { isMixedWorkflowSelection, multiSelectSteps } = useMultiSelectDerived(
    multiSelect.selectedIds,
    snapshots,
    effectiveSteps,
  );

  const handleCardClickOrSelect = useCallback(
    (task: Task) => {
      if (isMultiSelectMode) {
        toggleSelect(task.id);
        return;
      }
      handleCardClick(task);
    },
    [isMultiSelectMode, toggleSelect, handleCardClick],
  );

  const automation = useMoveErrorState(router);

  useWorkflowSelection({
    store,
    userSettings: hooks.userSettings,
    workspaceState,
    workflowsState,
    commitSettings: hooks.commitSettings,
    setActiveWorkflow,
    setWorkflows,
  });

  return {
    isMobile,
    kanbanViewMode,
    kanban,
    workspaceState,
    workflowsState,
    searchQuery,
    setSearchQuery,
    ...hooks,
    handleEdit: handleEditWithCleanup,
    ...automation,
    handleOpenTask,
    handleCardClick: handleCardClickOrSelect,
    multiSelect,
    isMixedWorkflowSelection,
    multiSelectSteps,
    effectiveWorkflowId,
    effectiveSteps,
    setMobileWorkflowFocusId,
  };
}

export function KanbanBoard({ onPreviewTask, onOpenTask, onBeforeEdit }: KanbanBoardProps = {}) {
  const s = useKanbanBoardSetup(onPreviewTask, onOpenTask, onBeforeEdit);
  const isMobileSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const setMobileSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);

  // Collapse search on unmount so the global flag doesn't auto-open (and focus)
  // the search bar after navigating to another route.
  useEffect(() => () => setMobileSearchOpen(false), [setMobileSearchOpen]);

  // Memoized so the dialog/child components don't see a new array identity on
  // every board re-render. Declared before the early return to keep hook order
  // stable.
  const stepOptions = useMemo(
    () =>
      s.effectiveSteps.map((step) => ({
        id: step.id,
        title: step.title,
        events: step.events,
      })),
    [s.effectiveSteps],
  );

  if (!s.isMounted) {
    return <div className="h-dvh w-full bg-background" />;
  }

  return (
    <div className="h-dvh w-full flex flex-col" data-testid="kanban-board">
      <HomepageCommands onCreateTask={s.handleCreate} />
      <KanbanHeader
        workspaceId={s.workspaceState.activeId ?? undefined}
        searchQuery={s.searchQuery}
        onSearchChange={s.setSearchQuery}
      />
      {s.isMobile && isMobileSearchOpen && (
        <MobileSearchBar searchQuery={s.searchQuery} onSearchChange={s.setSearchQuery} />
      )}
      <KanbanBoardDialogs
        isDialogOpen={s.isDialogOpen}
        handleDialogOpenChange={s.handleDialogOpenChange}
        workspaceId={s.workspaceState.activeId}
        workflowId={s.effectiveWorkflowId}
        defaultStepId={s.effectiveSteps[0]?.id ?? null}
        stepOptions={stepOptions}
        editingTask={s.editingTask}
        handleDialogSuccess={s.handleDialogSuccess}
        moveError={s.moveError}
        setMoveError={s.setMoveError}
        handleGoToTask={s.handleGoToTask}
      />
      <SwimlaneContainer
        viewMode={s.kanbanViewMode || ""}
        workflowFilter={s.workflowsState.activeId}
        onPreviewTask={s.handleCardClick}
        onOpenTask={s.handleOpenTask}
        onEditTask={s.handleEdit}
        onDeleteTask={s.handleDelete}
        onArchiveTask={s.handleArchive}
        onMoveError={s.handleMoveError}
        deletingTaskId={s.deletingTaskId}
        archivingTaskId={s.archivingTaskId}
        showMaximizeButton={s.enablePreviewOnClick}
        searchQuery={s.searchQuery}
        selectedRepositoryIds={s.userSettings.repositoryIds}
        selectedIds={s.multiSelect.selectedIds}
        onToggleSelect={s.multiSelect.toggleSelect}
        onSelectRange={s.multiSelect.selectRange}
        isMultiSelectMode={s.multiSelect.isMultiSelectMode}
        onToggleMultiSelect={s.multiSelect.toggleMultiSelect}
        onWorkflowChange={s.handleWorkflowChange}
        onMobileWorkflowFocusChange={s.setMobileWorkflowFocusId}
      />
      <TaskMultiSelectToolbar
        selectedIds={s.multiSelect.selectedIds}
        steps={s.multiSelectSteps}
        isProcessing={s.multiSelect.isProcessing}
        canMove={!s.isMixedWorkflowSelection}
        onClearSelection={s.multiSelect.clearSelection}
        onBulkDelete={s.multiSelect.bulkDelete}
        onBulkArchive={s.multiSelect.bulkArchive}
        onBulkMove={s.multiSelect.bulkMove}
      />
      {s.isMobile && <MobileFab onClick={s.handleCreate} />}
    </div>
  );
}

type KanbanBoardDialogsProps = {
  isDialogOpen: boolean;
  handleDialogOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  defaultStepId: string | null;
  stepOptions: Array<{
    id: string;
    title: string;
    events?: {
      on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
      on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    };
  }>;
  editingTask: Task | null;
  handleDialogSuccess: (task: BackendTask, mode: "create" | "edit") => void;
  moveError: MoveTaskError | null;
  setMoveError: (error: MoveTaskError | null) => void;
  handleGoToTask: () => void;
};

function KanbanBoardDialogs({
  isDialogOpen,
  handleDialogOpenChange,
  workspaceId,
  workflowId,
  defaultStepId,
  stepOptions,
  editingTask,
  handleDialogSuccess,
  moveError,
  setMoveError,
  handleGoToTask,
}: KanbanBoardDialogsProps) {
  return (
    <>
      <TaskCreateDialog
        open={isDialogOpen}
        onOpenChange={handleDialogOpenChange}
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={defaultStepId}
        steps={stepOptions}
        editingTask={
          editingTask
            ? {
                id: editingTask.id,
                title: editingTask.title,
                description: editingTask.description,
                workflowStepId: editingTask.workflowStepId,
                state: editingTask.state as BackendTask["state"],
                repositoryId: editingTask.repositoryId,
              }
            : null
        }
        onSuccess={handleDialogSuccess}
        initialValues={
          editingTask
            ? {
                title: editingTask.title,
                description: editingTask.description,
                state: editingTask.state as BackendTask["state"],
                repositoryId: editingTask.repositoryId,
              }
            : undefined
        }
        mode={editingTask ? "edit" : "create"}
      />
      <ApprovalWarningDialog
        moveError={moveError}
        setMoveError={setMoveError}
        handleGoToTask={handleGoToTask}
      />
    </>
  );
}

function ApprovalWarningDialog({
  moveError,
  setMoveError,
  handleGoToTask,
}: {
  moveError: MoveTaskError | null;
  setMoveError: (error: MoveTaskError | null) => void;
  handleGoToTask: () => void;
}) {
  return (
    <AlertDialog open={!!moveError} onOpenChange={(open) => !open && setMoveError(null)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="flex items-center gap-2">
            <IconAlertTriangle className="h-5 w-5 text-amber-500" />
            Approval Required
          </AlertDialogTitle>
          <AlertDialogDescription>{moveError?.message}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Dismiss</AlertDialogCancel>
          {moveError?.taskId && (
            <AlertDialogAction onClick={handleGoToTask}>Go to Task</AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
