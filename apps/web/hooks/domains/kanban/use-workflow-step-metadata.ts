import { useEffect, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { readJourneyWorkflowSteps } from "@/hooks/journey-metadata-resources";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { useWebSocketClient } from "@/lib/ws/connection";
import { workflowStepToState } from "@/lib/ssr/mapper";
import type { KanbanState } from "@/lib/state/slices";

type Steps = KanbanState["steps"];
const EMPTY_STEPS: Steps = [];

/** Missing detail metadata does not establish demand for the destination board's tasks. */
export function useWorkflowStepMetadata(workflowId: string | null) {
  const store = useAppStoreApi();
  const scope = useAppStore(stateReadScopeIdentity);
  const connection = useAppStore((state) => state.connection.status);
  const allowed = useAppStore(
    (state) =>
      (state.auth.mode === "disabled" || state.auth.authenticated) &&
      state.workflows.items.some(
        (workflow) =>
          workflow.id === workflowId && workflow.workspaceId === state.workspaces.activeId,
      ),
  );
  const client = useWebSocketClient();
  const [revision, setRevision] = useState(0);
  const key =
    allowed && workflowId ? JSON.stringify([scope, workflowId, connection, revision]) : null;
  const [result, setResult] = useState<{ key: string; steps: Steps } | null>(null);
  useEffect(() => {
    if (!client || !workflowId) return;
    const subscriptions = (
      ["workflow.step.created", "workflow.step.updated", "workflow.step.deleted"] as const
    ).map((action) =>
      client.on(action, (message) => {
        if (message.payload.step.workflow_id === workflowId) setRevision((value) => value + 1);
      }),
    );
    return () => subscriptions.forEach((unsubscribe) => unsubscribe());
  }, [client, workflowId]);
  useEffect(() => {
    if (!key || !workflowId) return;
    const controller = new AbortController();
    void readJourneyWorkflowSteps(store, workflowId, { signal: controller.signal })
      .then((response) => {
        if (!controller.signal.aborted)
          setResult({ key, steps: response.steps.map(workflowStepToState) });
      })
      .catch(() => {});
    return () => controller.abort();
  }, [key, workflowId, store]);
  return key && result?.key === key ? result.steps : EMPTY_STEPS;
}
