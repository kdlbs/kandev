"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskCRUD } from "@/hooks/use-task-crud";
import { qk } from "@/lib/query/keys";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { Task as BackendTask } from "@/lib/types/http";
import type { KanbanMultiData } from "@/lib/query/query-options/kanban";
import type { WorkflowsState } from "@/lib/state/slices";

type UseKanbanActionsOptions = {
  workspaceState: { activeId: string | null };
  workflowsState: WorkflowsState;
};

// ---------------------------------------------------------------------------
// Cache writers (update TQ multi-cache for immediate UI response)
// ---------------------------------------------------------------------------

/**
 * Upserts a newly-created task into every workflow snapshot in the TQ cache
 * where the task's workflowId matches. Falls back gracefully when the snapshot
 * for the target workflow hasn't loaded yet.
 */
function upsertCreatedTask(
  queryClient: ReturnType<typeof useQueryClient>,
  task: BackendTask,
): void {
  const repoId = task.repositories?.[0]?.repository_id ?? undefined;
  const kanbanTask = toKanbanTask({ ...task, repository_id: repoId });
  const wfId = task.workflow_id;

  queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
    if (!prev) return prev;
    const snap = prev.snapshots[wfId];
    if (!snap) return prev;
    const exists = snap.tasks.some((t) => t.id === task.id);
    const tasks = exists
      ? snap.tasks.map((t) => (t.id === task.id ? { ...t, repositoryId: repoId } : t))
      : [...snap.tasks, kanbanTask];
    return {
      ...prev,
      snapshots: { ...prev.snapshots, [wfId]: { ...snap, tasks } },
    };
  });
}

/**
 * Patches an edited task's dialog-editable fields (title, description,
 * repositoryId) in the TQ multi cache.
 */
function patchEditedTask(queryClient: ReturnType<typeof useQueryClient>, task: BackendTask): void {
  const repoId = task.repositories?.[0]?.repository_id ?? undefined;
  const wfId = task.workflow_id;

  queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
    if (!prev) return prev;
    const snap = prev.snapshots[wfId];
    if (!snap) return prev;
    const tasks = snap.tasks.map((t) =>
      t.id === task.id
        ? {
            ...t,
            title: task.title,
            description: task.description ?? undefined,
            repositoryId: repoId ?? t.repositoryId,
          }
        : t,
    );
    return {
      ...prev,
      snapshots: { ...prev.snapshots, [wfId]: { ...snap, tasks } },
    };
  });
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useKanbanActions({ workspaceState, workflowsState }: UseKanbanActionsOptions) {
  const router = useRouter();
  const store = useAppStoreApi();
  const queryClient = useQueryClient();
  const activeWorkspaceId = workspaceState.activeId;

  // CRUD operations from existing hook
  const {
    isDialogOpen,
    editingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleArchive,
    handleDialogOpenChange,
    setIsDialogOpen,
    setEditingTask,
    deletingTaskId,
    archivingTaskId,
  } = useTaskCRUD();

  // Handle task dialog success (create/update)
  const handleDialogSuccess = useCallback(
    (task: BackendTask, mode: "create" | "edit") => {
      if (mode === "create") {
        upsertCreatedTask(queryClient, task);
        if (task.workspace_id) {
          store.getState().invalidateRepositories(task.workspace_id);
        }
      } else {
        patchEditedTask(queryClient, task);
      }
    },
    [queryClient, store],
  );

  // Handle workspace change with navigation
  const handleWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      if (nextWorkspaceId === activeWorkspaceId) return;
      store.getState().setActiveWorkspace(nextWorkspaceId);
      if (nextWorkspaceId) {
        router.push(`/?workspaceId=${nextWorkspaceId}`);
      } else {
        router.push("/");
      }
    },
    [router, store, activeWorkspaceId],
  );

  // Handle workflow change with navigation
  const handleWorkflowChange = useCallback(
    (nextWorkflowId: string | null) => {
      if (nextWorkflowId === workflowsState.activeId) return;
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
    archivingTaskId,

    // CRUD actions
    handleCreate,
    handleEdit,
    handleDelete,
    handleArchive,
    handleDialogOpenChange,
    handleDialogSuccess,

    // Navigation actions
    handleWorkspaceChange,
    handleWorkflowChange,
  };
}

// Re-export useActiveWorkspaceId for consumers
export function useActiveWorkspaceId() {
  return useAppStore((s) => s.workspaces.activeId);
}
