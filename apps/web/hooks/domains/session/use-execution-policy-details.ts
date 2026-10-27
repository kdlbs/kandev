"use client";

import { useQuery } from "@tanstack/react-query";
import { useSandboxAccess } from "@/hooks/domains/settings/use-sandbox-access";
import { useTask } from "@/hooks/use-task";
import { fetchSandboxRevision, previewSandboxSelection } from "@/lib/api/domains/sandbox-api";
import type { ExecutionSnapshot } from "@/lib/types/execution";

export function useExecutionPolicyDetails(snapshot: ExecutionSnapshot | null) {
  const access = useSandboxAccess();
  const { context, complete } = useExecutionContext(snapshot);
  const canRead = access.enabled && access.canRead;
  const canPreview = canRead && access.canEdit && complete;
  const revisionId = snapshot?.sandbox?.revision_id ?? "";
  const revision = useQuery({
    queryKey: ["execution-policy", "captured", access.identity, revisionId],
    queryFn: ({ signal }) => fetchSandboxRevision(revisionId, { init: { signal } }),
    enabled: canRead && Boolean(revisionId),
    retry: false,
    gcTime: 0,
  });
  const preview = useQuery({
    queryKey: ["execution-policy", "preview", access.identity, context],
    queryFn: ({ signal }) => previewSandboxSelection(context, { init: { signal } }),
    enabled: canPreview,
    retry: false,
    gcTime: 0,
  });
  const captured =
    canRead &&
    revision.data?.id === revisionId &&
    revision.data.digest === snapshot?.sandbox?.policy_digest
      ? revision.data
      : undefined;
  return {
    access,
    canPreview,
    revision,
    captured,
    preview: {
      isError: preview.isError,
      isPending: preview.isPending,
      data: canPreview ? preview.data : undefined,
    },
  };
}

function useExecutionContext(snapshot: ExecutionSnapshot | null) {
  const task = useTask(snapshot?.task_id ?? null, false);
  const context = {
    workspace_id: task?.workspaceId,
    task_id: task?.id,
    workflow_step_id: task?.workflowStepId,
    executor_id: snapshot?.executor_id ?? "",
    agent_profile_id: snapshot?.agent_profile_id,
  };
  const complete = Boolean(
    snapshot &&
    task?.id === snapshot.task_id &&
    task.workspaceId &&
    task.workflowStepId &&
    context.executor_id,
  );
  return { context, complete };
}
