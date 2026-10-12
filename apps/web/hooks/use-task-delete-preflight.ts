import { useCallback, useEffect, useState } from "react";
import { getTaskDeletePreflight } from "@/lib/api";

export type TaskDeletePreflightStatus = "idle" | "loading" | "resolved" | "error";

export type TaskDeletePreflightResult = {
  status: TaskDeletePreflightStatus;
  requiresDiscardConsent: boolean;
  confirmationId: string;
  requestKey: string;
  scopeKey: string;
  retry: () => void;
};

type StoredPreflightResult = {
  key: string;
  status: TaskDeletePreflightStatus;
  requiresDiscardConsent: boolean;
  confirmationId: string;
};

export function useTaskDeletePreflight(
  open: boolean,
  taskId?: string,
  taskIds?: string[],
  cascade = false,
  discardWorktreeChanges = false,
): TaskDeletePreflightResult {
  const requestIds = taskIds ?? (taskId ? [taskId] : []);
  const scopeKey = JSON.stringify({ ids: requestIds, cascade });
  const idsKey = JSON.stringify({ ids: requestIds, cascade, discardWorktreeChanges });
  const [retryVersion, setRetryVersion] = useState(0);
  const requestKey = `${idsKey}:${retryVersion}`;
  const [result, setResult] = useState<StoredPreflightResult>({
    key: "",
    status: "idle",
    requiresDiscardConsent: false,
    confirmationId: "",
  });

  useEffect(() => {
    if (!open) {
      setResult({ key: "", status: "idle", requiresDiscardConsent: false, confirmationId: "" });
      return;
    }
    if (requestIds.length === 0) {
      setResult({
        key: requestKey,
        status: "error",
        requiresDiscardConsent: false,
        confirmationId: "",
      });
      return;
    }
    let cancelled = false;
    setResult({
      key: requestKey,
      status: "loading",
      requiresDiscardConsent: false,
      confirmationId: "",
    });
    getTaskDeletePreflight(requestIds, cascade, discardWorktreeChanges)
      .then((response) => {
        if (cancelled) return;
        setResult({
          key: requestKey,
          status: "resolved",
          requiresDiscardConsent: response.requires_discard_consent,
          confirmationId: response.confirmation_id,
        });
      })
      .catch(() => {
        if (!cancelled) {
          setResult({
            key: requestKey,
            status: "error",
            requiresDiscardConsent: false,
            confirmationId: "",
          });
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
    requestKey,
    scopeKey,
    retry,
  };
}
