'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useAppStoreApi } from '@/components/state-provider';
import { useTaskCRUD } from '@/hooks/use-task-crud';
import type { Task as BackendTask } from '@/lib/types/http';
import type { KanbanState, WorkspaceState, BoardState } from '@/lib/state/slices';

type UseKanbanActionsOptions = {
  kanban: KanbanState;
  workspaceState: WorkspaceState;
  boardsState: BoardState;
};

export function useKanbanActions({ kanban, workspaceState, boardsState }: UseKanbanActionsOptions) {
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
  } = useTaskCRUD();

  // Handle task dialog success (create/update)
  const handleDialogSuccess = useCallback(
    (task: BackendTask, mode: 'create' | 'edit') => {
      if (mode === 'create') {
        store.getState().hydrate({
          kanban: {
            ...kanban,
            tasks: [
              ...kanban.tasks,
              {
                id: task.id,
                workflowStepId: task.workflow_step_id,
                title: task.title,
                description: task.description ?? undefined,
                position: task.position ?? 0,
                state: task.state,
                repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
              },
            ],
          },
        });
        return;
      }
      store.getState().hydrate({
        kanban: {
          ...kanban,
          tasks: kanban.tasks.map((item: KanbanState['tasks'][number]) =>
            item.id === task.id
              ? {
                  ...item,
                  title: task.title,
                  description: task.description ?? undefined,
                  workflowStepId: task.workflow_step_id ?? item.workflowStepId,
                  position: task.position ?? item.position,
                  state: task.state ?? item.state,
                  repositoryId: task.repositories?.[0]?.repository_id ?? item.repositoryId,
                }
              : item
          ),
        },
      });
    },
    [kanban, store]
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
