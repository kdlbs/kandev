import { useCallback, useEffect, useState } from "react";
import type { TaskWorkspaceCleanup, DeleteTaskParams } from "@/lib/api/domains/kanban-api";
import { getTaskDeletePreflight } from "@/lib/api";

export type TaskDeletePreflightStatus = "idle" | "loading" | "resolved" | "error";

export type TaskDeletePreflightResult = {
  status: TaskDeletePreflightStatus;
  workspaceInspection?: "complete" | "unknown" | "not_requested";
  requiresDiscardConsent: boolean;
  confirmationId: string;
  requestKey: string;
  scopeKey: string;
  retry: () => void;
};

type StoredPreflightResult = {
  key: string;
  status: TaskDeletePreflightStatus;
  workspaceInspection?: "complete" | "unknown" | "not_requested";
  requiresDiscardConsent: boolean;
  confirmationId: string;
};

export function useTaskDeletePreflight(
  open: boolean,
  taskId?: string,
  taskIds?: string[],
  cascade = false,
  options: {
    discardWorktreeChanges?: boolean;
    workspaceCleanup?: TaskWorkspaceCleanup;
    allowUnverifiedWorkspaceChanges?: boolean;
  } = {},
): TaskDeletePreflightResult {
  const {
    discardWorktreeChanges = false,
    workspaceCleanup = "delete",
    allowUnverifiedWorkspaceChanges = false,
  } = options;
  const requestIds = taskIds ?? (taskId ? [taskId] : []);
  const scopeKey = JSON.stringify({ ids: requestIds, cascade });
  const idsKey = JSON.stringify({
    ids: requestIds,
    cascade,
    discardWorktreeChanges,
    workspaceCleanup,
    allowUnverifiedWorkspaceChanges,
  });
  const [retryVersion, setRetryVersion] = useState(0);
  const requestKey = `${idsKey}:${retryVersion}`;
  const [result, setResult] = useState<StoredPreflightResult>(emptyPreflight());

  useEffect(() => {
    if (!open) {
      setResult(emptyPreflight());
      return;
    }
    if (requestIds.length === 0) {
      setResult(emptyPreflight(requestKey, "error"));
      return;
    }
    let cancelled = false;
    setResult(emptyPreflight(requestKey, "loading"));
    requestPreflight(requestIds, cascade, {
      workspaceCleanup,
      discardWorktreeChanges,
      allowUnverifiedWorkspaceChanges,
    })
      .then((response) => {
        if (cancelled) return;
        setResult({
          key: requestKey,
          status: "resolved",
          requiresDiscardConsent: response.requires_discard_consent,
          confirmationId: response.confirmation_id,
          workspaceInspection: response.workspace_inspection,
        });
      })
      .catch(() => {
        if (!cancelled) {
          setResult(emptyPreflight(requestKey, "error"));
        }
      });
    return () => {
      cancelled = true;
    };
    // taskId / taskIds intentionally excluded; idsKey is their stable summary.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, requestKey]);

  const retry = useCallback(() => setRetryVersion((version) => version + 1), []);
  if (!open || result.key !== requestKey) {
    return {
      status: "idle",
      requiresDiscardConsent: false,
      confirmationId: "",
      requestKey,
      scopeKey,
      retry,
    };
  }
  return {
    status: result.status,
    requiresDiscardConsent: result.requiresDiscardConsent,
    confirmationId: result.confirmationId,
    workspaceInspection: result.workspaceInspection,
    requestKey,
    scopeKey,
    retry,
  };
}

function emptyPreflight(
  key = "",
  status: TaskDeletePreflightStatus = "idle",
): StoredPreflightResult {
  return { key, status, requiresDiscardConsent: false, confirmationId: "" };
}
function requestPreflight(ids: string[], cascade: boolean, options: DeleteTaskParams) {
  if (options.workspaceCleanup === "retain")
    return getTaskDeletePreflight(ids, cascade, false, undefined, { workspaceCleanup: "retain" });
  if (options.allowUnverifiedWorkspaceChanges)
    return getTaskDeletePreflight(ids, cascade, options.discardWorktreeChanges, undefined, {
      workspaceCleanup: "delete",
      allowUnverifiedWorkspaceChanges: true,
    });
  return getTaskDeletePreflight(ids, cascade, options.discardWorktreeChanges);
}
