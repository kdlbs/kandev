import { useCallback, useRef, useState } from "react";
import * as api from "@/lib/api/domains/archived-data-retention-api";
import type {
  ArchivedDataPolicy,
  ArchivedDataPolicyUpdate,
  ArchivedDataRetentionStatus,
} from "@/lib/types/archived-data-retention";
import {
  loadRetentionStatus,
  newRetentionLifetime,
  operationObserved,
  useAcceptedOperationRefresh,
  useRetentionActionSinks,
  useRetentionLifetime,
  useRetentionMutation,
  useRetentionStatusPolling,
} from "./use-retention-lifecycle";

export function useArchivedDataRetention() {
  const [status, setStatus] = useState<ArchivedDataRetentionStatus | null>(null);
  const [acceptedId, setAcceptedId] = useState<string | null>(null);
  const acceptedOperation = useRef<{ id: string; kind: "analysis" | "cleanup" } | null>(null);
  const sinks = useRetentionActionSinks();
  const owner = useRef(newRetentionLifetime());
  const reload = useCallback(
    () =>
      loadRetentionStatus(
        owner.current,
        api.fetchArchivedDataRetention,
        (next) => {
          setStatus(next);
          const accepted = acceptedOperation.current;
          if (!accepted || operationObserved(next, accepted)) {
            acceptedOperation.current = null;
            setAcceptedId(null);
          }
        },
        () => sinks.setStatusError(null),
        sinks.setStatusError,
      ),
    [sinks.setStatusError],
  );
  const refresh = useCallback(() => {
    sinks.setStatusError(null);
    return reload();
  }, [reload, sinks.setStatusError]);
  useAcceptedOperationRefresh(acceptedId, reload);
  useRetentionLifetime(owner, reload);
  const preparing =
    status?.preparation.state === "pending" || status?.preparation.state === "running";
  const active = Boolean(acceptedId || preparing || status?.operation?.state === "running");
  useRetentionStatusPolling(reload, active, preparing);
  const perform = useRetentionMutation(owner, sinks.setPending, sinks);
  const acceptStatus = useCallback((next: ArchivedDataRetentionStatus) => {
    acceptedOperation.current = null;
    setStatus(next);
    setAcceptedId(null);
  }, []);
  const acceptOperation = useCallback(
    (result: { operation_id: string }, kind: "analysis" | "cleanup") => {
      acceptedOperation.current = { id: result.operation_id, kind };
      setAcceptedId(result.operation_id);
    },
    [],
  );
  const save = useCallback(
    (policy: ArchivedDataPolicyUpdate) =>
      perform(() => api.saveArchivedDataRetention(policy), acceptStatus),
    [perform, acceptStatus],
  );
  const analyze = useCallback(
    (policy: ArchivedDataPolicy) =>
      perform(
        () => api.analyzeArchivedDataRetention(policy),
        (result) => acceptOperation(result, "analysis"),
      ),
    [perform, acceptOperation],
  );
  const run = useCallback(
    (revision: number) =>
      perform(
        () => api.runArchivedDataRetention(revision),
        (result) => acceptOperation(result, "cleanup"),
      ),
    [perform, acceptOperation],
  );
  const cancel = useCallback(
    (id: string) => perform(() => api.cancelArchivedDataRetention(id), acceptStatus),
    [perform, acceptStatus],
  );
  return {
    status,
    error: sinks.actionError ?? sinks.statusError,
    statusError: sinks.statusError,
    actionError: sinks.actionError,
    pending: sinks.pending,
    active,
    preparing,
    acceptedId,
    reload,
    refresh,
    save,
    analyze,
    run,
    cancel,
  };
}
