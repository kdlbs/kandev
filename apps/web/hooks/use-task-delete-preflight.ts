import { useCallback, useEffect, useState } from "react";
import { getTaskDeletePreflight } from "@/lib/api";

export type TaskDeletePreflightStatus = "idle" | "loading" | "resolved" | "error";

export type TaskDeletePreflightResult = {
  status: TaskDeletePreflightStatus;
  requiresDiscardConsent: boolean;
  requestKey: string;
  retry: () => void;
};

type StoredPreflightResult = {
  key: string;
  status: TaskDeletePreflightStatus;
  requiresDiscardConsent: boolean;
};

export function useTaskDeletePreflight(
  open: boolean,
  taskId?: string,
  taskIds?: string[],
  cascade = false,
): TaskDeletePreflightResult {
  const requestIds = taskIds ?? (taskId ? [taskId] : []);
  const idsKey = JSON.stringify({ ids: requestIds, cascade });
  const [retryVersion, setRetryVersion] = useState(0);
  const requestKey = `${idsKey}:${retryVersion}`;
  const [result, setResult] = useState<StoredPreflightResult>({
    key: "",
    status: "idle",
    requiresDiscardConsent: false,
  });

  useEffect(() => {
    if (!open) {
      setResult({ key: "", status: "idle", requiresDiscardConsent: false });
      return;
    }
    if (requestIds.length === 0) {
      setResult({ key: requestKey, status: "error", requiresDiscardConsent: false });
      return;
    }
    let cancelled = false;
    setResult({ key: requestKey, status: "loading", requiresDiscardConsent: false });
    getTaskDeletePreflight(requestIds, cascade)
      .then((response) => {
        if (cancelled) return;
        setResult({
          key: requestKey,
          status: "resolved",
          requiresDiscardConsent: response.requires_discard_consent,
        });
      })
      .catch(() => {
        if (!cancelled) {
          setResult({ key: requestKey, status: "error", requiresDiscardConsent: false });
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
    return { status: "idle", requiresDiscardConsent: false, requestKey, retry };
  }
  return {
    status: result.status,
    requiresDiscardConsent: result.requiresDiscardConsent,
    requestKey,
    retry,
  };
}
