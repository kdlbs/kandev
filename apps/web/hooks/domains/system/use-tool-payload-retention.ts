import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@/lib/api/client";
import * as api from "@/lib/api/domains/tool-payload-retention-api";
import type {
  ToolPayloadAge,
  ToolPayloadPolicyUpdate,
  ToolPayloadRetentionStatus,
} from "@/lib/types/tool-payload-retention";
import {
  invalidateBackupList,
  useBackupListScope,
  type BackupListScope,
} from "./backup-list-query";
import {
  observeBackupPreparation,
  type BackupPreparationCorrelation,
} from "./backup-preparation-correlation";

type Lifetime = {
  epoch: number;
  mounted: boolean;
  mutating: boolean;
  reading: Promise<void> | null;
  generation: number;
  scope: BackupListScope;
  preparation: BackupPreparationCorrelation;
};
type AcceptedOperation = { id: string; kind: "analysis" | "cleanup" };
type AcceptedOperationRef = { current: AcceptedOperation | null };
type LifetimeRef = { current: Lifetime };
type RetentionMutation = <T>(request: () => Promise<T>, accept: (value: T) => void) => Promise<T>;
type RetentionStatusOptions = {
  owner: LifetimeRef;
  acceptedOperation: AcceptedOperationRef;
  backupScope: ReturnType<typeof useBackupListScope>;
  queryClient: ReturnType<typeof useQueryClient>;
  setStatus: (value: ToolPayloadRetentionStatus | null) => void;
  setStatusError: (value: unknown) => void;
  setAcceptedId: (value: string | null) => void;
};
type Updates = {
  pending: (value: boolean) => void;
  actionError: (cause: unknown) => void;
  clearErrors: () => void;
};

function operationObserved(status: ToolPayloadRetentionStatus, accepted: AcceptedOperation) {
  if (status.operation?.id === accepted.id) return true;
  const latest = accepted.kind === "analysis" ? status.last_analysis : status.last_run;
  return latest != null;
}

function loadStatus(
  owner: Lifetime,
  accept: (value: ToolPayloadRetentionStatus) => void,
  recover: () => void,
  fail: (cause: unknown) => void,
) {
  if (owner.mutating) return Promise.resolve();
  if (owner.reading) return owner.reading;
  const { epoch, generation } = owner;
  const current = () => owner.mounted && owner.epoch === epoch && owner.generation === generation;
  const request = api
    .fetchToolPayloadRetention()
    .then((next) => {
      if (current()) {
        accept(next);
        recover();
      }
    })
    .catch((cause: unknown) => {
      if (current()) fail(cause);
    })
    .finally(() => {
      if (owner.reading === request) owner.reading = null;
    });
  owner.reading = request;
  return request;
}
async function performMutation<T>(
  owner: Lifetime,
  request: () => Promise<T>,
  accept: (value: T) => void,
  updates: Updates,
) {
  // i18n-exempt: machine-only conflict; the card renders a translated error category.
  if (owner.mutating) throw new ApiError("busy", 409, { code: "busy" });
  owner.mutating = true;
  const mutationGeneration = ++owner.generation;
  owner.reading = null;
  const { epoch } = owner;
  const current = () =>
    owner.mounted && owner.epoch === epoch && owner.generation === mutationGeneration;
  updates.pending(true);
  updates.clearErrors();
  try {
    const result = await request();
    if (current()) accept(result);
    return result;
  } catch (cause) {
    if (current()) updates.actionError(cause);
    throw cause;
  } finally {
    if (current()) {
      owner.mutating = false;
      updates.pending(false);
    }
  }
}
function statusPollingInterval(active: boolean, preparing: boolean) {
  if (preparing) return 2000;
  if (active) return 5000;
  return 30000;
}
function useStatusPolling(reload: () => Promise<void>, active: boolean, preparing: boolean) {
  const interval = statusPollingInterval(active, preparing);
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      await reload();
      if (!stopped) timer = setTimeout(poll, interval);
    };
    timer = setTimeout(poll, interval);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [interval, reload]);
}
function useAcceptedOperationRefresh(acceptedId: string | null, reload: () => Promise<void>) {
  useEffect(() => {
    if (!acceptedId) return;
    const timer = setTimeout(() => void reload(), 0);
    return () => clearTimeout(timer);
  }, [acceptedId, reload]);
}
function useRetentionLifetime(
  owner: { current: Lifetime },
  reload: () => Promise<void>,
  scopeKey: string,
) {
  useEffect(() => {
    const lifetime = owner.current;
    lifetime.mounted = true;
    void reload();
    return () => {
      lifetime.mounted = false;
      lifetime.epoch++;
      lifetime.reading = null;
    };
  }, [owner, reload, scopeKey]);
}
function useRetentionMutation(
  owner: { current: Lifetime },
  setPending: (value: boolean) => void,
  setStatusError: (value: unknown) => void,
  setActionError: (value: unknown) => void,
) {
  return useCallback(
    <T>(request: () => Promise<T>, accept: (value: T) => void) =>
      performMutation(owner.current, request, accept, {
        pending: setPending,
        actionError: setActionError,
        clearErrors: () => {
          setStatusError(null);
          setActionError(null);
        },
      }),
    [owner, setActionError, setPending, setStatusError],
  );
}
function useRetentionOwner(
  backupScope: ReturnType<typeof useBackupListScope>,
  acceptedOperation: AcceptedOperationRef,
  resetState: () => void,
): LifetimeRef {
  const owner = useRef<Lifetime | null>(null);
  if (owner.current === null) {
    owner.current = {
      epoch: 0,
      mounted: false,
      mutating: false,
      reading: null,
      generation: 0,
      scope: backupScope.captureScope(),
      preparation: { activeRevision: null, settledRevision: -1 },
    };
  }
  const lifetime = owner as { current: Lifetime };
  useLayoutEffect(() => {
    const current = lifetime.current;
    if (
      current.scope.identityKey === backupScope.identityKey &&
      current.scope.generation === backupScope.generation
    ) {
      return;
    }
    current.scope = backupScope.captureScope();
    current.epoch++;
    current.generation++;
    current.mutating = false;
    current.reading = null;
    current.preparation = { activeRevision: null, settledRevision: -1 };
    acceptedOperation.current = null;
    resetState();
  }, [
    acceptedOperation,
    backupScope.captureScope,
    backupScope.generation,
    backupScope.identityKey,
    lifetime,
    resetState,
  ]);
  return lifetime;
}

function useRetentionStatus({
  owner,
  acceptedOperation,
  backupScope,
  queryClient,
  setStatus,
  setStatusError,
  setAcceptedId,
}: RetentionStatusOptions) {
  const acceptStatus = useCallback(
    (next: ToolPayloadRetentionStatus, saveCandidateRevision?: number) => {
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
          backupScope.isCurrentScope(settledScope),
        );
      }
    },
    [acceptedOperation, backupScope.isCurrentScope, owner, queryClient, setAcceptedId, setStatus],
  );
  const reload = useCallback(
    () => loadStatus(owner.current, acceptStatus, () => setStatusError(null), setStatusError),
    [acceptStatus, owner, setStatusError],
  );
  const refresh = useCallback(() => {
    setStatusError(null);
    return reload();
  }, [reload]);
  return { acceptStatus, reload, refresh };
}

function useRetentionOperations(
  backupScope: ReturnType<typeof useBackupListScope>,
  perform: RetentionMutation,
  acceptedOperation: AcceptedOperationRef,
  acceptStatus: (status: ToolPayloadRetentionStatus, saveCandidateRevision?: number) => void,
  setAcceptedId: (value: string | null) => void,
) {
  const acceptOperation = useCallback(
    (result: { operation_id: string }, kind: AcceptedOperation["kind"]) => {
      acceptedOperation.current = { id: result.operation_id, kind };
      setAcceptedId(result.operation_id);
    },
    [acceptedOperation, setAcceptedId],
  );
  const save = useCallback(
    (policy: ToolPayloadPolicyUpdate, options?: { backupChoiceAttempt?: boolean }) => {
      const writerScope = backupScope.captureScope();
      const candidateRevision =
        options?.backupChoiceAttempt && policy.backup_choice === "backup"
          ? policy.revision + 1
          : undefined;
      return perform(
        () => api.saveToolPayloadRetention(policy),
        (next) => {
          if (!backupScope.isCurrentScope(writerScope)) return;
          acceptedOperation.current = null;
          setAcceptedId(null);
          acceptStatus(next, candidateRevision);
        },
      );
    },
    [
      acceptStatus,
      acceptedOperation,
      backupScope.captureScope,
      backupScope.isCurrentScope,
      perform,
      setAcceptedId,
    ],
  );
  const analyze = useCallback(
    (age: ToolPayloadAge) =>
      perform(
        () => api.analyzeToolPayloadRetention(age),
        (result) => acceptOperation(result, "analysis"),
      ),
    [perform, acceptOperation],
  );
  const run = useCallback(
    (revision: number) =>
      perform(
        () => api.runToolPayloadRetention(revision),
        (result) => acceptOperation(result, "cleanup"),
      ),
    [perform, acceptOperation],
  );
  const cancel = useCallback(
    (id: string) => {
      const writerScope = backupScope.captureScope();
      return perform(
        () => api.cancelToolPayloadRetention(id),
        (next) => {
          if (!backupScope.isCurrentScope(writerScope)) return;
          acceptedOperation.current = null;
          setAcceptedId(null);
          acceptStatus(next);
        },
      );
    },
    [
      acceptStatus,
      acceptedOperation,
      backupScope.captureScope,
      backupScope.isCurrentScope,
      perform,
      setAcceptedId,
    ],
  );
  return { save, analyze, run, cancel };
}

export function useToolPayloadRetention() {
  const backupScope = useBackupListScope();
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<ToolPayloadRetentionStatus | null>(null);
  const [statusError, setStatusError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  const [acceptedId, setAcceptedId] = useState<string | null>(null);
  const acceptedOperation = useRef<AcceptedOperation | null>(null);
  const resetState = useCallback(() => {
    setStatus(null);
    setStatusError(null);
    setActionError(null);
    setPending(false);
    setAcceptedId(null);
  }, []);
  const owner = useRetentionOwner(backupScope, acceptedOperation, resetState);
  const { acceptStatus, reload, refresh } = useRetentionStatus({
    owner,
    acceptedOperation,
    backupScope,
    queryClient,
    setStatus,
    setStatusError,
    setAcceptedId,
  });
  useAcceptedOperationRefresh(acceptedId, reload);
  useRetentionLifetime(owner, reload, backupScope.identityKey);
  const preparing =
    status?.preparation.state === "pending" || status?.preparation.state === "running";
  const active = Boolean(acceptedId || preparing || status?.operation?.state === "running");
  useStatusPolling(reload, active, preparing);
  const perform = useRetentionMutation(owner, setPending, setStatusError, setActionError);
  const operations = useRetentionOperations(
    backupScope,
    perform,
    acceptedOperation,
    acceptStatus,
    setAcceptedId,
  );
  return {
    status,
    error: actionError ?? statusError,
    statusError,
    actionError,
    pending,
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
