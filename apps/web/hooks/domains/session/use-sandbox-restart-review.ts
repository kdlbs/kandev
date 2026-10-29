"use client";

import { useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { previewSandboxSelection } from "@/lib/api/domains/sandbox-api";
import {
  getSessionRestartReviewStore,
  settleSessionRestartReview,
  type SessionRestartReview,
} from "@/lib/session/session-restart-review";
import { useExecutionPolicyDetails } from "./use-execution-policy-details";
import { useSessionExecution } from "./use-session-execution";

import type { SandboxSelectionContext } from "@/lib/types/sandbox";

type ReviewError = "stale" | "unavailable" | null;

function isCurrent(owner: ReturnType<typeof useAppStoreApi>, request: SessionRestartReview) {
  const state = owner.getState();
  const session = state.taskSessions.items[request.sessionId];
  return (
    getSessionRestartReviewStore(owner).getState() === request &&
    JSON.stringify(state.auth) === request.identity &&
    state.connection.status === "connected" &&
    session?.task_id === request.taskId &&
    session.execution_snapshot?.sandbox?.attempt_id === request.attemptId &&
    (request.navigationRevision === undefined ||
      state.taskRemoval.navigationRevision === request.navigationRevision)
  );
}

async function readRestartPreview(
  request: SessionRestartReview,
  context: SandboxSelectionContext,
  signal: AbortSignal,
  stillCurrent: () => boolean,
) {
  const task = await fetchTask(request.taskId, { cache: "no-store", init: { signal } });
  if (!stillCurrent() || task.id !== request.taskId) return null;
  return previewSandboxSelection(
    {
      ...context,
      workspace_id: task.workspace_id,
      task_id: task.id,
      workflow_step_id: task.workflow_step_id || undefined,
    },
    { init: { signal } },
  );
}

export function useSandboxRestartReview(request: SessionRestartReview) {
  const owner = useAppStoreApi();
  useAppStore((state) => state.auth);
  useAppStore((state) => state.connection.status);
  useAppStore((state) => state.taskRemoval.navigationRevision);
  const { session } = useSessionExecution(request.sessionId);
  const snapshot = session?.execution_snapshot ?? null;
  const details = useExecutionPolicyDetails(snapshot);
  const [error, setError] = useState<ReviewError>(null);
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  const lifecycle = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const valid = isCurrent(owner, request) && details.access.enabled && details.access.canRead;
  const displayed = details.preview.data;
  const canUseCurrent = Boolean(
    valid &&
    details.canPreview &&
    !details.preview.isFetching &&
    displayed?.resolved &&
    !displayed.resolution_error &&
    displayed.selection_digest,
  );
  const cancel = () => {
    controller.current?.abort();
    settleSessionRestartReview(owner, request, null);
  };

  useEffect(() => {
    if (!valid) settleSessionRestartReview(owner, request, null);
  }, [owner, request, valid]);
  useEffect(() => {
    const generation = ++lifecycle.current;
    return () => {
      controller.current?.abort();
      queueMicrotask(() => {
        if (lifecycle.current === generation) settleSessionRestartReview(owner, request, null);
      });
    };
  }, [owner, request]);

  const confirmCurrent = async () => {
    if (pending.current || !canUseCurrent || !isCurrent(owner, request)) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    const abort = new AbortController();
    controller.current = abort;
    try {
      const current = await readRestartPreview(request, details.context, abort.signal, () =>
        isCurrent(owner, request),
      );
      if (!current) {
        cancel();
        return;
      }
      if (!isCurrent(owner, request)) {
        cancel();
        return;
      }
      if (
        !current.resolved ||
        current.resolution_error ||
        current.selection_digest !== displayed?.selection_digest
      ) {
        setError("stale");
        await details.refreshContext();
        await details.preview.refetch();
        return;
      }
      settleSessionRestartReview(owner, request, {
        mode: "current",
        selectionDigest: current.selection_digest!,
      });
    } catch {
      if (isCurrent(owner, request)) setError("unavailable");
    } finally {
      pending.current = false;
      controller.current = null;
      if (getSessionRestartReviewStore(owner).getState() === request) setBusy(false);
    }
  };
  const confirmCaptured = () => {
    if (!pending.current && valid && isCurrent(owner, request)) {
      settleSessionRestartReview(owner, request, { mode: "captured" });
    }
  };
  return {
    details,
    snapshot,
    name: session?.name || request.sessionId,
    valid,
    canUseCurrent,
    busy,
    error,
    cancel,
    confirmCurrent,
    confirmCaptured,
  };
}
