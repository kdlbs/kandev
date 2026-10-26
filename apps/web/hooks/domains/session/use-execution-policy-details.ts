"use client";

import { useQuery } from "@tanstack/react-query";
import { useSandboxAccess } from "@/hooks/domains/settings/use-sandbox-access";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { fetchSandboxRevision, previewSandboxSelection } from "@/lib/api/domains/sandbox-api";
import type { ExecutionSnapshot } from "@/lib/types/execution";

export function useExecutionPolicyDetails(snapshot: ExecutionSnapshot | null) {
  const access = useSandboxAccess();
  const { context, complete, refreshContext } = useExecutionContext(snapshot, access);
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
    context,
    refreshContext,
    revision,
    captured,
    preview: {
      refetch: preview.refetch,
      isError: preview.isError,
      isPending: preview.isPending,
      isFetching: preview.isFetching,
      data: canPreview ? preview.data : undefined,
    },
  };
}

function useExecutionContext(
  snapshot: ExecutionSnapshot | null,
  access: ReturnType<typeof useSandboxAccess>,
) {
  const task = useQuery({
    queryKey: ["execution-policy", "task-context", access.identity, snapshot?.task_id],
    queryFn: ({ signal }) => fetchTask(snapshot!.task_id, { cache: "no-store", init: { signal } }),
    enabled: access.enabled && access.canEdit && Boolean(snapshot?.task_id),
    retry: false,
    gcTime: 0,
  });
  const current = access.canEdit ? task.data : undefined;
  const { context, complete } = executionPreviewContext(current, snapshot);
  return { context, complete, refreshContext: task.refetch };
}

function executionPreviewContext(
  current: Awaited<ReturnType<typeof fetchTask>> | undefined,
  snapshot: ExecutionSnapshot | null,
) {
  const context = {
    workspace_id: current?.workspace_id,
    task_id: current?.id,
    workflow_step_id: current?.workflow_step_id || undefined,
    executor_id: snapshot?.executor_id ?? "",
    agent_profile_id: snapshot?.agent_profile_id,
  };
  const complete = Boolean(
    snapshot && current?.id === snapshot.task_id && current.workspace_id && context.executor_id,
  );
  return { context, complete };
}
