import { useEffect, useRef } from 'react';
import { fetchWorkflowSnapshot } from '@/lib/api';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { KanbanState } from '@/lib/state/slices/kanban/types';

type KanbanTask = KanbanState['tasks'][number];

export function useAllWorkflowSnapshots(workspaceId: string | null) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const workflows = useAppStore((state) => state.workflows.items);
  const lastFetchedRef = useRef<string>('');

  useEffect(() => {
    if (!workspaceId) {
      return;
    }

    const workspaceWorkflows = workflows.filter((w) => w.workspaceId === workspaceId);
    if (workspaceWorkflows.length === 0) {
      return;
    }

    // Deduplicate: skip if same set of workflow IDs already fetched for this connection status
    const key = workspaceWorkflows.map((w) => w.id).sort().join(',') + ':' + connectionStatus;
    if (lastFetchedRef.current === key) {
      return;
    }
    lastFetchedRef.current = key;

    const { setKanbanMultiLoading, setWorkflowSnapshot } = store.getState();
    setKanbanMultiLoading(true);

    Promise.all(
      workspaceWorkflows.map(async (wf) => {
        try {
          const snapshot = await fetchWorkflowSnapshot(wf.id, { cache: 'no-store' });

          const steps = snapshot.steps.map((step) => ({
            id: step.id,
            title: step.name,
            color: step.color ?? 'bg-neutral-400',
            position: step.position,
            events: step.events,
          }));
          const stepIds = new Set(steps.map((s) => s.id));

          const tasks: KanbanTask[] = snapshot.tasks
            .map((task) => {
              const workflowStepId = task.workflow_step_id;
              if (!workflowStepId || !stepIds.has(workflowStepId)) return null;

              return {
                id: task.id,
                workflowStepId,
                title: task.title,
                description: task.description ?? undefined,
                position: task.position ?? 0,
                state: task.state,
                repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
                primarySessionId: task.primary_session_id ?? undefined,
                sessionCount: task.session_count ?? undefined,
                reviewStatus: task.review_status ?? undefined,
                updatedAt: task.updated_at,
              } as KanbanTask;
            })
            .filter((t): t is KanbanTask => t !== null);

          setWorkflowSnapshot(wf.id, {
            workflowId: wf.id,
            workflowName: wf.name,
            steps,
            tasks,
          });
        } catch (err) {
          console.error(`[useAllWorkflowSnapshots] Failed to fetch snapshot for workflow "${wf.name}" (${wf.id}):`, err);
        }
      })
    ).finally(() => {
      store.getState().setKanbanMultiLoading(false);
    });
  }, [workspaceId, workflows, connectionStatus, store]);
}
