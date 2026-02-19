"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { useAppStoreApi } from "@/components/state-provider";
import { useTaskCRUD } from "@/hooks/use-task-crud";
import type { Task as BackendTask } from "@/lib/types/http";
import type { KanbanState, WorkspaceState, WorkflowsState } from "@/lib/state/slices";

type UseKanbanActionsOptions = {
  workspaceState: WorkspaceState;
  workflowsState: WorkflowsState;
};

type KanbanTask = KanbanState["tasks"][number];

/** Handle creating a new task in the kanban board, merging with any WS-provided data. */
function hydrateCreatedTask(
  store: ReturnType<typeof useAppStoreApi>,
  task: BackendTask,
  currentKanban: KanbanState,
) {
  const repoId = task.repositories?.[0]?.repository_id ?? undefined;
  const existing = currentKanban.tasks.find((t: KanbanTask) => t.id === task.id);
  if (existing) {
    if (repoId && !existing.repositoryId) {
      store.getState().hydrate({
        kanban: {
          ...currentKanban,
          tasks: currentKanban.tasks.map((t: KanbanTask) =>
            t.id === task.id ? { ...t, repositoryId: repoId } : t,
          ),
        },
      });
    }
  } else {
    store.getState().hydrate({
      kanban: {
        ...currentKanban,
        tasks: [
          ...currentKanban.tasks,
          {
            id: task.id,
            workflowStepId: task.workflow_step_id,
            title: task.title,
            description: task.description ?? undefined,
            position: task.position ?? 0,
            state: task.state,
            repositoryId: repoId,
          },
        ],
      },
    });
  }
  if (task.workspace_id) {
    store.getState().invalidateRepositories(task.workspace_id);
  }
}

/** Handle editing an existing task - only update dialog-editable fields. */
function hydrateEditedTask(
  store: ReturnType<typeof useAppStoreApi>,
  task: BackendTask,
  currentKanban: KanbanState,
) {
  store.getState().hydrate({
    kanban: {
      ...currentKanban,
      tasks: currentKanban.tasks.map((item: KanbanTask) =>
        item.id === task.id
          ? {
              ...item,
              title: task.title,
              description: task.description ?? undefined,
              repositoryId: task.repositories?.[0]?.repository_id ?? item.repositoryId,
            }
          : item,
      ),
    },
  });
}

export function useKanbanActions({ workspaceState, workflowsState }: UseKanbanActionsOptions) {
  const router = useRouter();
  const store = useAppStoreApi();

  // CRUD operations from existing hook
  const {
    isDialogOpen,
    editingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleDialogOpenChange,
    setIsDialogOpen,
    setEditingTask,
    deletingTaskId,
  } = useTaskCRUD();

  // Handle task dialog success (create/update)
  // Read current kanban state at call time (not from closure) to avoid
  // overwriting WebSocket-driven updates that arrived while the dialog was open.
  const handleDialogSuccess = useCallback(
    (task: BackendTask, mode: "create" | "edit") => {
      const currentKanban = store.getState().kanban;
      if (mode === "create") {
        hydrateCreatedTask(store, task, currentKanban);
        return;
      }
      hydrateEditedTask(store, task, currentKanban);
    },
    [store],
  );

  // Handle workspace change with navigation
  const handleWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      if (nextWorkspaceId === workspaceState.activeId) {
        return;
      }
      store.getState().setActiveWorkspace(nextWorkspaceId);
      if (nextWorkspaceId) {
        router.push(`/?workspaceId=${nextWorkspaceId}`);
      } else {
        router.push("/");
      }
    },
    [router, store, workspaceState.activeId],
  );

  // Handle workflow change with navigation
  const handleWorkflowChange = useCallback(
    (nextWorkflowId: string | null) => {
      if (nextWorkflowId === workflowsState.activeId) {
        return;
      }
      store.getState().setActiveWorkflow(nextWorkflowId);
      if (nextWorkflowId) {
        const workspaceId = workflowsState.items.find(
          (workflow: WorkflowsState["items"][number]) => workflow.id === nextWorkflowId,
        )?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : "";
        router.push(`/?workflowId=${nextWorkflowId}${workspaceParam}`);
      }
    },
    [router, store, workflowsState.activeId, workflowsState.items],
  );

  return {
    // CRUD state
    isDialogOpen,
    editingTask,
    setIsDialogOpen,
    setEditingTask,
    deletingTaskId,

    // CRUD actions
    handleCreate,
    handleEdit,
    handleDelete,
    handleDialogOpenChange,
    handleDialogSuccess,

    // Navigation actions
    handleWorkspaceChange,
    handleWorkflowChange,
  };
}
