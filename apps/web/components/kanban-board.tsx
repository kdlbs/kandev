"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Task } from "./kanban-card";
import { TaskCreateDialog } from "./task-create-dialog";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { Task as BackendTask } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import { type WorkflowAutomation, type MoveTaskError } from "@/hooks/use-drag-and-drop";
import { SwimlaneContainer } from "./kanban/swimlane-container";
import { KanbanHeader } from "./kanban/kanban-header";
import { useKanbanData, useKanbanActions, useKanbanNavigation } from "@/hooks/domains/kanban";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { HomepageCommands } from "./homepage-commands";
import { getWebSocketClient } from "@/lib/ws/connection";
import { linkToSession } from "@/lib/links";
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

    const desiredWorkflowId =
      settings.workflowId &&
      workspaceWorkflows.some(
        (workflow: WorkflowsState["items"][number]) => workflow.id === settings.workflowId,
      )
        ? settings.workflowId
        : null;
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
    workspaceState.activeId,
  ]);
}

function useWorkflowAutomationState(router: ReturnType<typeof useRouter>) {
  const [workflowAutomation, setWorkflowAutomation] = useState<WorkflowAutomation | null>(null);
  const [isWorkflowDialogOpen, setIsWorkflowDialogOpen] = useState(false);
  const [moveError, setMoveError] = useState<MoveTaskError | null>(null);

  const handleWorkflowAutomation = useCallback((automation: WorkflowAutomation) => {
    setWorkflowAutomation(automation);
    setIsWorkflowDialogOpen(true);
  }, []);

  const handleMoveError = useCallback((error: MoveTaskError) => {
    setMoveError(error);
  }, []);

  const handleGoToTask = useCallback(() => {
    if (moveError?.sessionId) {
      router.push(linkToSession(moveError.sessionId));
    }
    setMoveError(null);
  }, [moveError, router]);

  const handleWorkflowSessionCreate = useCallback(
    async (data: {
      prompt: string;
      agentProfileId: string;
      executorId: string;
      environmentId: string;
    }) => {
      if (!workflowAutomation) return;
      const client = getWebSocketClient();
      if (!client) return;
      try {
        await client.request(
          "orchestrator.start",
          {
            task_id: workflowAutomation.taskId,
            agent_profile_id: data.agentProfileId,
            executor_id: data.executorId,
            prompt: data.prompt.trim(),
            workflow_step_id: workflowAutomation.workflowStep.id,
          },
          15000,
        );
      } catch (err) {
        console.error("Failed to start session for workflow step:", err);
      } finally {
        setWorkflowAutomation(null);
        setIsWorkflowDialogOpen(false);
      }
    },
    [workflowAutomation],
  );

  return {
    workflowAutomation,
    setWorkflowAutomation,
    isWorkflowDialogOpen,
    setIsWorkflowDialogOpen,
    moveError,
    setMoveError,
    handleWorkflowAutomation,
    handleMoveError,
    handleGoToTask,
    handleWorkflowSessionCreate,
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
  onOpenTask?: (task: Task, sessionId: string) => void;
}

function useKanbanBoardHooks(searchQuery: string, workspaceState: ReturnType<typeof useKanbanBoardStore>["workspaceState"], workflowsState: ReturnType<typeof useKanbanBoardStore>["workflowsState"]) {
  const {
    isDialogOpen,
    editingTask,
    setIsDialogOpen,
    setEditingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleDialogOpenChange,
    handleDialogSuccess,
    handleWorkspaceChange,
    handleWorkflowChange,
    deletingTaskId,
  } = useKanbanActions({ workspaceState, workflowsState });
  const {
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    activeSteps,
    isMounted,
    setTaskSessionAvailability,
  } = useKanbanData({
    onWorkspaceChange: handleWorkspaceChange,
    onWorkflowChange: handleWorkflowChange,
    searchQuery,
  });
  return {
    isDialogOpen, editingTask, setIsDialogOpen, setEditingTask,
    handleCreate, handleEdit, handleDelete, handleDialogOpenChange,
    handleDialogSuccess, deletingTaskId,
    enablePreviewOnClick, userSettings, commitSettings, activeSteps,
    isMounted, setTaskSessionAvailability,
  };
}

function useKanbanBoardSetup(
  onPreviewTask: KanbanBoardProps["onPreviewTask"],
  onOpenTask: KanbanBoardProps["onOpenTask"],
) {
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();
  const [searchQuery, setSearchQuery] = useState("");
  const {
    store, kanbanViewMode, kanban, workspaceState, workflowsState,
    setActiveWorkflow, setWorkflows,
  } = useKanbanBoardStore();

  useAllWorkflowSnapshots(workspaceState.activeId);

  const hooks = useKanbanBoardHooks(searchQuery, workspaceState, workflowsState);
  const { handleOpenTask, handleCardClick } = useKanbanNavigation({
    enablePreviewOnClick: hooks.enablePreviewOnClick,
    isMobile,
    onPreviewTask,
    onOpenTask,
    setEditingTask: hooks.setEditingTask,
    setIsDialogOpen: hooks.setIsDialogOpen,
    setTaskSessionAvailability: hooks.setTaskSessionAvailability,
  });
  const automation = useWorkflowAutomationState(router);

  useWorkflowSelection({
    store, userSettings: hooks.userSettings, workspaceState, workflowsState,
    commitSettings: hooks.commitSettings, setActiveWorkflow, setWorkflows,
  });

  return {
    kanbanViewMode, kanban, workspaceState, workflowsState,
    searchQuery, setSearchQuery,
    ...hooks, ...automation,
    handleOpenTask, handleCardClick,
  };
}

export function KanbanBoard({ onPreviewTask, onOpenTask }: KanbanBoardProps = {}) {
  const s = useKanbanBoardSetup(onPreviewTask, onOpenTask);

  if (!s.isMounted) {
    return <div className="h-dvh w-full bg-background" />;
  }

  const stepOptions = s.activeSteps.map((step) => ({
    id: step.id,
    title: step.title,
    events: step.events,
  }));

  return (
    <div className="h-dvh w-full flex flex-col">
      <HomepageCommands onCreateTask={s.handleCreate} />
      <KanbanHeader
        onCreateTask={s.handleCreate}
        workspaceId={s.workspaceState.activeId ?? undefined}
        searchQuery={s.searchQuery}
        onSearchChange={s.setSearchQuery}
      />
      <KanbanBoardDialogs
        isDialogOpen={s.isDialogOpen}
        handleDialogOpenChange={s.handleDialogOpenChange}
        workspaceId={s.workspaceState.activeId}
        workflowId={s.kanban.workflowId}
        defaultStepId={s.activeSteps[0]?.id ?? null}
        stepOptions={stepOptions}
        editingTask={s.editingTask}
        handleDialogSuccess={s.handleDialogSuccess}
        isWorkflowDialogOpen={s.isWorkflowDialogOpen}
        setIsWorkflowDialogOpen={s.setIsWorkflowDialogOpen}
        setWorkflowAutomation={s.setWorkflowAutomation}
        workflowAutomation={s.workflowAutomation}
        handleWorkflowSessionCreate={s.handleWorkflowSessionCreate}
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
        onMoveError={s.handleMoveError}
        onWorkflowAutomation={s.handleWorkflowAutomation}
        deletingTaskId={s.deletingTaskId}
        searchQuery={s.searchQuery}
        selectedRepositoryIds={s.userSettings.repositoryIds}
      />
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
  isWorkflowDialogOpen: boolean;
  setIsWorkflowDialogOpen: (open: boolean) => void;
  setWorkflowAutomation: (automation: WorkflowAutomation | null) => void;
  workflowAutomation: WorkflowAutomation | null;
  handleWorkflowSessionCreate: (data: {
    prompt: string;
    agentProfileId: string;
    executorId: string;
    environmentId: string;
  }) => Promise<void>;
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
  isWorkflowDialogOpen,
  setIsWorkflowDialogOpen,
  setWorkflowAutomation,
  workflowAutomation,
  handleWorkflowSessionCreate,
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
      <TaskCreateDialog
        open={isWorkflowDialogOpen}
        onOpenChange={(open) => {
          setIsWorkflowDialogOpen(open);
          if (!open) setWorkflowAutomation(null);
        }}
        mode="session"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={workflowAutomation?.workflowStep.id ?? null}
        steps={stepOptions}
        editingTask={null}
        onCreateSession={handleWorkflowSessionCreate}
        initialValues={{
          title: "",
          description: workflowAutomation?.taskDescription ?? "",
        }}
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
          {moveError?.sessionId && (
            <AlertDialogAction onClick={handleGoToTask}>Go to Task</AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
