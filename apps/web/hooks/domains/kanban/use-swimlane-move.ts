import { useCallback } from 'react';
import { useAppStoreApi } from '@/components/state-provider';
import { useTaskActions } from '@/hooks/use-task-actions';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Task } from '@/components/kanban-card';
import type { WorkflowAutomation, MoveTaskError } from '@/hooks/use-drag-and-drop';
import type { KanbanState } from '@/lib/state/slices/kanban/types';

function hasAutoStartEvent(response: { workflow_step?: { events?: { on_enter?: Array<{ type: string }> } } }): boolean {
  return response?.workflow_step?.events?.on_enter?.some((a) => a.type === 'auto_start_agent') ?? false;
}

async function triggerAutoStart(taskId: string, sessionId: string, workflowStepId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) return;
  try {
    await client.request('orchestrator.start', { task_id: taskId, session_id: sessionId, workflow_step_id: workflowStepId }, 15000);
  } catch (err) {
    console.error('Failed to auto-start session for workflow step:', err);
  }
}

export function useSwimlaneMove(
  workflowId: string,
  opts: {
    onMoveError?: (error: MoveTaskError) => void;
    onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  }
) {
  const store = useAppStoreApi();
  const { moveTaskById } = useTaskActions();

  const moveTask = useCallback(
    async (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId) return;

      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const targetTasks = snapshot.tasks
        .filter(
          (t: KanbanState['tasks'][number]) =>
            t.workflowStepId === targetStepId && t.id !== task.id
        )
        .sort(
          (a: KanbanState['tasks'][number], b: KanbanState['tasks'][number]) =>
            a.position - b.position
        );
      const nextPosition = targetTasks.length;

      const originalTasks = snapshot.tasks;

      // Optimistic update
      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t: KanbanState['tasks'][number]) =>
          t.id === task.id
            ? { ...t, workflowStepId: targetStepId, position: nextPosition }
            : t
        ),
      });

      try {
        const response = await moveTaskById(task.id, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });

        if (hasAutoStartEvent(response)) {
          const sessionId = task.primarySessionId ?? null;
          if (sessionId) {
            await triggerAutoStart(task.id, sessionId, response.workflow_step.id);
          } else {
            opts.onWorkflowAutomation?.({
              taskId: task.id,
              sessionId: null,
              workflowStep: response.workflow_step,
              taskDescription: task.description ?? '',
            });
          }
        }
      } catch (error) {
        // Rollback
        const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
        if (currentSnapshot) {
          store.getState().setWorkflowSnapshot(workflowId, {
            ...currentSnapshot,
            tasks: originalTasks,
          });
        }
        const message = error instanceof Error ? error.message : 'Failed to move task';
        opts.onMoveError?.({
          message,
          taskId: task.id,
          sessionId: task.primarySessionId ?? null,
        });
      }
    },
    [workflowId, store, moveTaskById, opts]
  );

  return { moveTask };
}
