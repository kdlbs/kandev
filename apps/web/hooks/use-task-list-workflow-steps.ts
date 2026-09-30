import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Task, Workflow } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { useWorkflowOptionPreviews } from "@/hooks/use-workflow-option-previews";
import { useWebSocketClient } from "@/lib/ws/connection";

export function useTaskListWorkflowSteps(
  workspaceId: string | null,
  tasks: Task[],
  workflows: Workflow[],
  enabled: boolean,
) {
  const [revision, setRevision] = useState(0);
  const connection = useAppStore((state) => state.connection.status);
  const client = useWebSocketClient();
  const authorizedIds = new Set(
    workflows
      .filter((workflow) => workflow.workspace_id === workspaceId)
      .map((workflow) => workflow.id),
  );
  const workflowIds = [
    ...new Set(
      tasks
        .filter(
          (task) =>
            task.workspace_id === workspaceId &&
            task.workflow_step_id &&
            authorizedIds.has(task.workflow_id),
        )
        .map((task) => task.workflow_id),
    ),
  ].sort();
  const workflowIdsKey = JSON.stringify(workflowIds);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  useEffect(() => {
    if (!client || !enabled) return;
    const ids = new Set<string>(JSON.parse(workflowIdsKey));
    const actions = [
      "workflow.step.created",
      "workflow.step.updated",
      "workflow.step.deleted",
    ] as const;
    const subscriptions = actions.map((action) =>
      client.on(action, (message) => {
        if (ids.has(message.payload.step.workflow_id)) refresh();
      }),
    );
    return () => subscriptions.forEach((unsubscribe) => unsubscribe());
  }, [client, enabled, workflowIdsKey, refresh]);
  const { previews } = useWorkflowOptionPreviews(
    workspaceId,
    enabled,
    workflowIds,
    `${connection}:${revision}`,
  );
  return { previews, refresh };
}

export function useTasksListStepRefresh(
  {
    activeWorkspaceId: workspaceId,
    workflows,
    fetchTasks,
  }: {
    activeWorkspaceId: string | null;
    workflows: Workflow[];
    fetchTasks: (reset?: boolean) => Promise<void>;
  },
  tasks: Task[],
  group: string,
) {
  const enabled = group === "workflow_step";
  const metadata = useTaskListWorkflowSteps(workspaceId, tasks, workflows, enabled);
  useForegroundRefresh(() => fetchTasks(true), Boolean(workspaceId), workspaceId);
  useForegroundRefresh(metadata.refresh, enabled, workspaceId);
  const refresh = async () => {
    metadata.refresh();
    await fetchTasks();
  };
  return { previews: metadata.previews, refresh };
}
