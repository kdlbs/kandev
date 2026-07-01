"use client";

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@/lib/routing/client-router";
import { useAppStoreApi } from "@/components/state-provider";
import { useAllCachedWorkflows } from "@/hooks/use-workflow-cache";
import { useTaskCRUD } from "@/hooks/use-task-crud";
import { qk } from "@/lib/query/keys";
import { updateWorkflowSnapshotQuery } from "@/lib/query/workflow-snapshot-cache";
import type { Task as BackendTask } from "@/lib/types/http";
import type { WorkspaceState, WorkflowsState } from "@/lib/state/slices";

type UseKanbanActionsOptions = {
  workspaceState: WorkspaceState;
  workflowsState: WorkflowsState;
};

function isTaskDetailPath(): boolean {
  if (typeof window === "undefined") return false;
  return /^\/(?:t|tasks)\/[^/]+\/?$/.test(window.location.pathname);
}

function kanbanWorkspaceHref(workspaceId: string): string {
  if (typeof window === "undefined") return `/?workspaceId=${workspaceId}`;
  const current = new URLSearchParams(window.location.search);
  const next = new URLSearchParams();
  const workflowId = current.get("workflowId");
  if (workflowId) next.set("workflowId", workflowId);
  next.set("workspaceId", workspaceId);
  return `/?${next.toString()}`;
}

/** Handle creating a new task in the kanban board, merging with any WS-provided data. */
function hydrateCreatedTask(queryClient: ReturnType<typeof useQueryClient>, task: BackendTask) {
  queryClient.setQueryData(qk.tasks.detail(task.id), task);
  updateWorkflowSnapshotQuery(queryClient, task.workflow_id, (snapshot) => {
    const existing = snapshot.tasks.find((item) => item.id === task.id);
    return {
      ...snapshot,
      tasks: existing
        ? snapshot.tasks.map((item) => (item.id === task.id ? { ...item, ...task } : item))
        : [...snapshot.tasks, task],
    };
  });
}

/** Handle editing an existing task - only update dialog-editable fields. */
function hydrateEditedTask(queryClient: ReturnType<typeof useQueryClient>, task: BackendTask) {
  queryClient.setQueryData(qk.tasks.detail(task.id), (current: BackendTask | undefined) => ({
    ...(current ?? task),
    title: task.title,
    description: task.description,
    repositories: task.repositories,
  }));
  updateWorkflowSnapshotQuery(queryClient, task.workflow_id, (snapshot) => {
    if (!snapshot.tasks.some((item) => item.id === task.id)) return snapshot;
    return {
      ...snapshot,
      tasks: snapshot.tasks.map((item) =>
        item.id === task.id
          ? {
              ...item,
              title: task.title,
              description: task.description,
              repositories: task.repositories,
            }
          : item,
      ),
    };
  });
}

export function useKanbanActions({ workspaceState, workflowsState }: UseKanbanActionsOptions) {
  const router = useRouter();
  const store = useAppStoreApi();
  const queryClient = useQueryClient();
  const workflows = useAllCachedWorkflows();

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
  // Read current kanban state at call time (not from closure) to avoid
  // overwriting WebSocket-driven updates that arrived while the dialog was open.
  const handleDialogSuccess = useCallback(
    (task: BackendTask, mode: "create" | "edit") => {
      if (mode === "create") {
        hydrateCreatedTask(queryClient, task);
        return;
      }
      hydrateEditedTask(queryClient, task);
    },
    [queryClient],
  );

  // Handle workspace change with navigation
  const handleWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      if (nextWorkspaceId === workspaceState.activeId) {
        return;
      }
      store.getState().setActiveWorkspace(nextWorkspaceId);
      if (isTaskDetailPath()) {
        return;
      }
      if (nextWorkspaceId) {
        router.push(kanbanWorkspaceHref(nextWorkspaceId));
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
      if (isTaskDetailPath()) {
        return;
      }
      if (nextWorkflowId) {
        const workspaceId = workflows.find(
          (workflow) => workflow.id === nextWorkflowId,
        )?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : "";
        router.push(`/?workflowId=${nextWorkflowId}${workspaceParam}`);
      }
    },
    [router, store, workflows, workflowsState.activeId],
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
