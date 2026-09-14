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
type Updates = { pending: (value: boolean) => void; error: (cause: unknown) => void };

function loadStatus(
  owner: Lifetime,
  accept: (value: ToolPayloadRetentionStatus) => void,
  fail: Updates["error"],
) {
  if (owner.mutating) return Promise.resolve();
  if (owner.reading) return owner.reading;
  const { epoch, generation } = owner;
  const current = () => owner.mounted && owner.epoch === epoch && owner.generation === generation;
  const request = api
    .fetchToolPayloadRetention()
    .then((next) => {
      if (current()) accept(next);
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
  updates.error(null);
  try {
    const result = await request();
    if (current()) accept(result);
    return result;
  } catch (cause) {
    if (current()) updates.error(cause);
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
export function useToolPayloadRetention() {
  const [status, setStatus] = useState<ToolPayloadRetentionStatus | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  const [acceptedId, setAcceptedId] = useState<string | null>(null);
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
          // A post-command read is authoritative even if another administrator
          // has already replaced the completed operation in bounded history.
          setAcceptedId(null);
        },
        setError,
      ),
    [],
  );
  const refresh = useCallback(() => {
    setError(null);
    return reload();
  }, [reload]);
  useEffect(() => {
    const lifetime = owner.current;
    lifetime.mounted = true;
    void reload();
    return () => {
      lifetime.mounted = false;
      lifetime.epoch++;
      lifetime.reading = null;
    };
  }, [reload]);
  const preparing =
    status?.preparation.state === "pending" || status?.preparation.state === "running";
  const active = Boolean(acceptedId || preparing || status?.operation?.state === "running");
  useStatusPolling(reload, active, preparing);
  const perform = useCallback(
    <T>(request: () => Promise<T>, accept: (value: T) => void) =>
      performMutation(owner.current, request, accept, { pending: setPending, error: setError }),
    [],
  );
  const acceptStatus = useCallback((next: ToolPayloadRetentionStatus) => {
    setStatus(next);
    setAcceptedId(null);
  }, []);
  const acceptOperation = useCallback(
    (result: { operation_id: string }) => setAcceptedId(result.operation_id),
    [],
  );
  const save = useCallback(
    (policy: ToolPayloadPolicyUpdate) =>
      perform(() => api.saveToolPayloadRetention(policy), acceptStatus),
    [perform, acceptStatus],
  );
  const analyze = useCallback(
    (age: ToolPayloadAge) => perform(() => api.analyzeToolPayloadRetention(age), acceptOperation),
    [perform, acceptOperation],
  );
  const run = useCallback(
    (revision: number) => perform(() => api.runToolPayloadRetention(revision), acceptOperation),
    [perform, acceptOperation],
  );
  const cancel = useCallback(
    (id: string) => perform(() => api.cancelToolPayloadRetention(id), acceptStatus),
    [perform, acceptStatus],
  );
  return {
    status,
    error,
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
