import { useCallback, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import * as api from "@/lib/api/domains/archived-data-retention-api";
import type {
  ArchivedDataPolicy,
  ArchivedDataPolicyUpdate,
  ArchivedDataRetentionStatus,
} from "@/lib/types/archived-data-retention";
import { invalidateBackupList, useBackupListScope } from "./backup-list-query";
import { observeBackupPreparation } from "./backup-preparation-correlation";
import {
  loadRetentionStatus,
  operationObserved,
  useAcceptedOperationRefresh,
  useRetentionActionSinks,
  useRetentionLifetime,
  useRetentionMutation,
  useRetentionScopeOwner,
  useRetentionStatusPolling,
  type RetentionAcceptedOperation,
  type RetentionLifetime,
} from "./use-retention-lifecycle";

type BackupScope = ReturnType<typeof useBackupListScope>;
type AcceptedRef = { current: RetentionAcceptedOperation | null };

// Accepting a status also correlates a saved backup choice with its preparation:
// once the prepared backup settles, the backup list of the scope that saved it is
// refreshed, so the new backup is listed without waiting for the list to go stale.
function useAcceptRetentionStatus(
  owner: { current: RetentionLifetime },
  acceptedOperation: AcceptedRef,
  backupScope: BackupScope,
  setStatus: (value: ArchivedDataRetentionStatus) => void,
  setAcceptedId: (value: string | null) => void,
) {
  const queryClient = useQueryClient();
  const { isCurrentScope } = backupScope;
  return useCallback(
    (next: ArchivedDataRetentionStatus, saveCandidateRevision?: number) => {
      const current = owner.current;
      const observed = observeBackupPreparation(current.preparation, next, saveCandidateRevision);
      current.preparation = observed.correlation;
      setStatus(next);
      const accepted = acceptedOperation.current;
      if (!accepted || operationObserved(next, accepted)) {
        acceptedOperation.current = null;
        setAcceptedId(null);
      }
      if (observed.shouldInvalidate) {
        const settledScope = current.scope;
        void invalidateBackupList(queryClient, settledScope.identity, () =>
          isCurrentScope(settledScope),
        );
      }
    },
    [acceptedOperation, isCurrentScope, owner, queryClient, setAcceptedId, setStatus],
  );
}

type Perform = ReturnType<typeof useRetentionMutation>;

// Save and cancel replace the status. A response that lands after the backend
// identity changed belongs to the previous scope and is dropped.
function useRetentionOperations(
  backupScope: BackupScope,
  perform: Perform,
  acceptedOperation: AcceptedRef,
  acceptStatus: (next: ArchivedDataRetentionStatus, saveCandidateRevision?: number) => void,
  setAcceptedId: (value: string | null) => void,
) {
  const { captureScope, isCurrentScope } = backupScope;
  const replaceStatus = useCallback(
    (writerScope: ReturnType<typeof captureScope>, saveCandidateRevision?: number) =>
      (next: ArchivedDataRetentionStatus) => {
        if (!isCurrentScope(writerScope)) return;
        acceptedOperation.current = null;
        setAcceptedId(null);
        acceptStatus(next, saveCandidateRevision);
      },
    [acceptStatus, acceptedOperation, isCurrentScope, setAcceptedId],
  );
  const acceptOperation = useCallback(
    (result: { operation_id: string }, kind: RetentionAcceptedOperation["kind"]) => {
      acceptedOperation.current = { id: result.operation_id, kind };
      setAcceptedId(result.operation_id);
    },
    [acceptedOperation, setAcceptedId],
  );
  const save = useCallback(
    (policy: ArchivedDataPolicyUpdate, options?: { backupChoiceAttempt?: boolean }) => {
      const candidateRevision =
        options?.backupChoiceAttempt && policy.backup_choice === "backup"
          ? policy.revision + 1
          : undefined;
      return perform(
        () => api.saveArchivedDataRetention(policy),
        replaceStatus(captureScope(), candidateRevision),
      );
    },
    [captureScope, perform, replaceStatus],
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
    (id: string) =>
      perform(() => api.cancelArchivedDataRetention(id), replaceStatus(captureScope())),
    [captureScope, perform, replaceStatus],
  );
  return { save, analyze, run, cancel };
}

export function useArchivedDataRetention() {
  const backupScope = useBackupListScope();
  const [status, setStatus] = useState<ArchivedDataRetentionStatus | null>(null);
  const [acceptedId, setAcceptedId] = useState<string | null>(null);
  const acceptedOperation = useRef<RetentionAcceptedOperation | null>(null);
  const sinks = useRetentionActionSinks();
  const { setPending, setStatusError, setActionError } = sinks;
  const resetState = useCallback(() => {
    setStatus(null);
    setStatusError(null);
    setActionError(null);
    setPending(false);
    setAcceptedId(null);
  }, [setActionError, setPending, setStatusError]);
  const owner = useRetentionScopeOwner(backupScope, acceptedOperation, resetState);
  const acceptStatus = useAcceptRetentionStatus(
    owner,
    acceptedOperation,
    backupScope,
    setStatus,
    setAcceptedId,
  );
  const reload = useCallback(
    () =>
      loadRetentionStatus(
        owner.current,
        api.fetchArchivedDataRetention,
        (next) => acceptStatus(next),
        () => setStatusError(null),
        setStatusError,
      ),
    [acceptStatus, owner, setStatusError],
  );
  const refresh = useCallback(() => {
    setStatusError(null);
    return reload();
  }, [reload, setStatusError]);
  useAcceptedOperationRefresh(acceptedId, reload);
  useRetentionLifetime(owner, reload, backupScope.identityKey);
  const preparing =
    status?.preparation.state === "pending" || status?.preparation.state === "running";
  const active = Boolean(acceptedId || preparing || status?.operation?.state === "running");
  useRetentionStatusPolling(reload, active, preparing);
  const perform = useRetentionMutation(owner, setPending, sinks);
  const operations = useRetentionOperations(
    backupScope,
    perform,
    acceptedOperation,
    acceptStatus,
    setAcceptedId,
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
    captureScope: backupScope.captureScope,
    isCurrentScope: backupScope.isCurrentScope,
    reload,
    refresh,
    ...operations,
  };
}
