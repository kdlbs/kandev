import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import * as api from "@/lib/api/domains/tool-payload-retention-api";
import type {
  ToolPayloadAge,
  ToolPayloadPolicyUpdate,
  ToolPayloadRetentionStatus,
} from "@/lib/types/tool-payload-retention";

type Lifetime = {
  epoch: number;
  mounted: boolean;
  mutating: boolean;
  reading: Promise<void> | null;
  generation: number;
};
type AcceptedOperation = { id: string; kind: "analysis" | "cleanup" };
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
  owner.generation++;
  owner.reading = null;
  const { epoch } = owner;
  const current = () => owner.mounted && owner.epoch === epoch;
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
    owner.mutating = false;
    if (current()) updates.pending(false);
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
function useRetentionLifetime(owner: { current: Lifetime }, reload: () => Promise<void>) {
  useEffect(() => {
    const lifetime = owner.current;
    lifetime.mounted = true;
    void reload();
    return () => {
      lifetime.mounted = false;
      lifetime.epoch++;
      lifetime.reading = null;
    };
  }, [owner, reload]);
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
export function useToolPayloadRetention() {
  const [status, setStatus] = useState<ToolPayloadRetentionStatus | null>(null);
  const [statusError, setStatusError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  const [acceptedId, setAcceptedId] = useState<string | null>(null);
  const acceptedOperation = useRef<AcceptedOperation | null>(null);
  const owner = useRef<Lifetime>({
    epoch: 0,
    mounted: false,
    mutating: false,
    reading: null,
    generation: 0,
  });
  const reload = useCallback(
    () =>
      loadStatus(
        owner.current,
        (next) => {
          setStatus(next);
          const accepted = acceptedOperation.current;
          if (!accepted || operationObserved(next, accepted)) {
            acceptedOperation.current = null;
            setAcceptedId(null);
          }
        },
        () => setStatusError(null),
        setStatusError,
      ),
    [],
  );
  const refresh = useCallback(() => {
    setStatusError(null);
    setActionError(null);
    return reload();
  }, [reload]);
  useAcceptedOperationRefresh(acceptedId, reload);
  useRetentionLifetime(owner, reload);
  const preparing =
    status?.preparation.state === "pending" || status?.preparation.state === "running";
  const active = Boolean(acceptedId || preparing || status?.operation?.state === "running");
  useStatusPolling(reload, active, preparing);
  const perform = useRetentionMutation(owner, setPending, setStatusError, setActionError);
  const acceptStatus = useCallback((next: ToolPayloadRetentionStatus) => {
    acceptedOperation.current = null;
    setStatus(next);
    setAcceptedId(null);
  }, []);
  const acceptOperation = useCallback(
    (result: { operation_id: string }, kind: AcceptedOperation["kind"]) => {
      acceptedOperation.current = { id: result.operation_id, kind };
      setAcceptedId(result.operation_id);
    },
    [],
  );
  const save = useCallback(
    (policy: ToolPayloadPolicyUpdate) =>
      perform(() => api.saveToolPayloadRetention(policy), acceptStatus),
    [perform, acceptStatus],
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
    (id: string) => perform(() => api.cancelToolPayloadRetention(id), acceptStatus),
    [perform, acceptStatus],
  );
  return {
    status,
    error: actionError ?? statusError,
    pending,
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
