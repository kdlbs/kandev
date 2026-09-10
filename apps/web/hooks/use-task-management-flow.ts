import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { resolveTaskMenuTarget, type TaskMenuIdentity } from "@/lib/tasks/task-menu-target";

export type TaskManagementStage = "closed" | "menu" | "archive" | "delete" | "link";

export function useTaskManagementFlow() {
  const store = useAppStoreApi();
  const [identity, setIdentity] = useState<TaskMenuIdentity | null>(null);
  const [stage, setStage] = useState<TaskManagementStage>("closed");
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const kanban = useAppStore((state) => state.kanban);
  const allWorkflows = useAppStore((state) => state.workflows.items);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const task = useMemo(
    () => (identity ? resolveTaskMenuTarget(store.getState(), identity) : null),
    // Snapshot changes refresh metadata, but never replace the captured identity.
    [identity, store, snapshots, kanban, allWorkflows, workspaceId],
  );
  const getTarget = useCallback(
    () => (identity ? resolveTaskMenuTarget(store.getState(), identity) : null),
    [identity, store],
  );
  const open = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const workspaceId = state.workspaces.activeId;
      if (!workspaceId) return;
      const next = { taskId, workspaceId };
      if (!resolveTaskMenuTarget(state, next)) return;
      setIdentity(next);
      setStage("menu");
    },
    [store],
  );
  const close = useCallback(() => setStage("closed"), []);
  useEffect(() => {
    if (!task) close();
  }, [task, close]);
  const workflows = allWorkflows.filter(
    (workflow) => workflow.workspaceId === identity?.workspaceId,
  );
  const stepsByWorkflowId = Object.fromEntries(
    workflows.map((workflow) => [
      workflow.id,
      snapshots[workflow.id]?.steps ?? (kanban.workflowId === workflow.id ? kanban.steps : []),
    ]),
  );
  return {
    identity,
    task,
    stage: task ? stage : ("closed" as const),
    setStage,
    getTarget,
    open,
    close,
    workflows,
    stepsByWorkflowId,
  };
}
