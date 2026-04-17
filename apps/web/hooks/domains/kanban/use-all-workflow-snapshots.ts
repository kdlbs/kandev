import { useEffect, useRef } from "react";
import { fetchWorkflowSnapshot } from "@/lib/api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { Task } from "@/lib/types/http";

type KanbanTask = KanbanState["tasks"][number];

// eslint-disable-next-line complexity -- pure field mapping, no real branching logic
function mapSnapshotTask(task: Task, stepIds: Set<string>): KanbanTask | null {
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
    primarySessionState: task.primary_session_state ?? undefined,
    sessionCount: task.session_count ?? undefined,
    reviewStatus: task.review_status ?? undefined,
    primaryExecutorId: task.primary_executor_id ?? undefined,
    primaryExecutorType: task.primary_executor_type ?? undefined,
    primaryExecutorName: task.primary_executor_name ?? undefined,
    isRemoteExecutor: task.is_remote_executor ?? false,
    parentTaskId: task.parent_id ?? undefined,
    updatedAt: task.updated_at,
    createdAt: task.created_at,
  } as KanbanTask;
}

export function useAllWorkflowSnapshots(workspaceId: string | null) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const workflows = useAppStore((state) => state.workflows.items);
  const lastFetchedRef = useRef<string>("");

  useEffect(() => {
    if (!workspaceId) {
      return;
    }

    const workspaceWorkflows = workflows.filter((w) => w.workspaceId === workspaceId);
    if (workspaceWorkflows.length === 0) {
      return;
    }

    // Deduplicate: skip if same set of workflow IDs already fetched for this connection status
    const key =
      workspaceWorkflows
        .map((w) => w.id)
        .sort()
        .join(",") +
      ":" +
      connectionStatus;
    if (lastFetchedRef.current === key) {
      return;
    }
    lastFetchedRef.current = key;

    const { setKanbanMultiLoading, setWorkflowSnapshot } = store.getState();
    setKanbanMultiLoading(true);

    Promise.all(
      workspaceWorkflows.map(async (wf) => {
        try {
          const snapshot = await fetchWorkflowSnapshot(wf.id, { cache: "no-store" });

          const steps = snapshot.steps.map((step) => ({
            id: step.id,
            title: step.name,
            color: step.color ?? "bg-neutral-400",
            position: step.position,
            events: step.events,
            allow_manual_move: step.allow_manual_move,
            prompt: step.prompt,
            is_start_step: step.is_start_step,
          }));
          const stepIds = new Set(steps.map((s) => s.id));

          // Preserve runtime fields (e.g., primarySessionId) from existing
          // snapshot tasks when the fresh API response omits them. The backend
          // uses omitempty for session fields, so a task whose session was just
          // created may not have primary_session_id in the snapshot response
          // yet, even though a WS task.updated event already delivered it.
          const existingSnapshot = store.getState().kanbanMulti.snapshots[wf.id];
          const existingById = new Map((existingSnapshot?.tasks ?? []).map((t) => [t.id, t]));

          const tasks: KanbanTask[] = snapshot.tasks
            .filter((task) => !task.is_ephemeral) // Filter out ephemeral tasks (e.g., quick chat)
            .map((task) => {
              const mapped = mapSnapshotTask(task, stepIds);
              if (!mapped) return null;
              const existing = existingById.get(mapped.id);
              if (existing) {
                mapped.primarySessionId = mapped.primarySessionId || existing.primarySessionId;
                mapped.primarySessionState =
                  mapped.primarySessionState || existing.primarySessionState;
              }
              return mapped;
            })
            .filter((t): t is KanbanTask => t !== null);

          setWorkflowSnapshot(wf.id, {
            workflowId: wf.id,
            workflowName: wf.name,
            steps,
            tasks,
          });
        } catch (err) {
          console.error(
            `[useAllWorkflowSnapshots] Failed to fetch snapshot for workflow "${wf.name}" (${wf.id}):`,
            err,
          );
        }
      }),
    ).finally(() => {
      store.getState().setKanbanMultiLoading(false);
    });
  }, [workspaceId, workflows, connectionStatus, store]);
}
