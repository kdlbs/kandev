'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useAppStoreApi } from '@/components/state-provider';
import { useTaskCRUD } from '@/hooks/use-task-crud';
import type { Task as BackendTask } from '@/lib/types/http';
import type { KanbanState, WorkspaceState, BoardState } from '@/lib/state/slices';

type UseKanbanActionsOptions = {
  workspaceState: WorkspaceState;
  boardsState: BoardState;
};

export function useKanbanActions({ workspaceState, boardsState }: UseKanbanActionsOptions) {
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
    (task: BackendTask, mode: 'create' | 'edit') => {
      const currentKanban = store.getState().kanban;
      if (mode === 'create') {
        const repoId = task.repositories?.[0]?.repository_id ?? undefined;
        const existing = currentKanban.tasks.find((t: KanbanState['tasks'][number]) => t.id === task.id);
        if (existing) {
          // WebSocket may have added the task already but without repositoryId.
          // Merge in any missing fields from the API response.
          if (repoId && !existing.repositoryId) {
            store.getState().hydrate({
              kanban: {
                ...currentKanban,
                tasks: currentKanban.tasks.map((t: KanbanState['tasks'][number]) =>
                  t.id === task.id ? { ...t, repositoryId: repoId } : t
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
        // Invalidate workspace repos cache so newly created repos show up in the edit dialog
        if (task.workspace_id) {
          store.getState().invalidateRepositories(task.workspace_id);
        }
        return;
      }
      // Only update fields that the edit dialog changes (title, description, repositories).
      // State and workflowStepId are managed by the backend via WebSocket events
      // (e.g. orchestrator.start moves the task to the auto-start column).
      store.getState().hydrate({
        kanban: {
          ...currentKanban,
          tasks: currentKanban.tasks.map((item: KanbanState['tasks'][number]) =>
            item.id === task.id
              ? {
                  ...item,
                  title: task.title,
                  description: task.description ?? undefined,
                  repositoryId: task.repositories?.[0]?.repository_id ?? item.repositoryId,
                }
              : item
          ),
        },
      });
    },
    [store]
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
        router.push('/');
      }
    },
    [router, store, workspaceState.activeId]
  );

  // Handle board change with navigation
  const handleBoardChange = useCallback(
    (nextBoardId: string | null) => {
      if (nextBoardId === boardsState.activeId) {
        return;
      }
      store.getState().setActiveBoard(nextBoardId);
      if (nextBoardId) {
        const workspaceId = boardsState.items.find((board: BoardState['items'][number]) => board.id === nextBoardId)?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : '';
        router.push(`/?boardId=${nextBoardId}${workspaceParam}`);
      }
    },
    [router, store, boardsState.activeId, boardsState.items]
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
    handleBoardChange,
  };
}
