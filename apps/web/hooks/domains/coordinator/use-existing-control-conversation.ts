import { useEffect, useState } from "react";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import type { Task } from "@/lib/types/http";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";

function matchesOriginalConversation(
  task: Task,
  workspaceId: string,
  taskId: string,
  coordinatorId?: string,
) {
  if (task.id !== taskId || task.workspace_id !== workspaceId) return false;
  if (coordinatorId)
    return (
      task.origin === "coordinator" &&
      task.metadata?.coordinator_id === coordinatorId &&
      task.metadata?.coordinator_purpose !== "dream"
    );
  return (
    task.is_ephemeral === true &&
    !task.workflow_id &&
    task.origin !== "coordinator" &&
    task.origin !== "automation_run" &&
    typeof task.metadata?.["kandev.plugin_id"] !== "string" &&
    task.metadata?.config_mode !== true
  );
}

/** Resolve only an existing task; no prepare, conversation-open, or recovery request. */
export function useExistingControlConversation(
  workspaceId: string,
  coordinatorId: string | undefined,
  taskId: string,
) {
  const store = useAppStoreApi();
  const identity = useAppStore(stateReadScopeIdentity);
  const key = JSON.stringify([identity, workspaceId, coordinatorId, taskId]);
  const [result, setResult] = useState<{ key: string; task: Task | null; error: boolean }>();
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setResult(undefined);
    fetchTask(taskId, { cache: "no-store", init: { signal: controller.signal } })
      .then((task) => {
        if (!active || stateReadScopeIdentity(store.getState()) !== identity) return;
        const matches = matchesOriginalConversation(task, workspaceId, taskId, coordinatorId);
        setResult({ key, task: matches ? task : null, error: !matches });
      })
      .catch(() => {
        if (active && stateReadScopeIdentity(store.getState()) === identity)
          setResult({ key, task: null, error: true });
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [workspaceId, coordinatorId, taskId, key, identity, store]);
  const current = result?.key === key ? result : undefined;
  const sessions = useTaskSessions(current?.task?.id ?? null);
  return { task: current?.task, taskError: current?.error, taskLoading: !current, ...sessions };
}
