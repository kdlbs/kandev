import { useCallback, useEffect, useState } from "react";
import { ApiError } from "@/lib/api/client";

// Shared status/mutation lifecycle for the install-wide retention policies. The
// two policies own different records and different shapes, but their read,
// mutation, and polling semantics are one contract, so a policy that grew its
// own copy would be able to drift from the governance it must match.

export type RetentionLifetime = {
  epoch: number;
  mounted: boolean;
  mutating: boolean;
  reading: Promise<void> | null;
  generation: number;
};

export type RetentionAcceptedOperation = { id: string; kind: "analysis" | "cleanup" };

export type RetentionErrorSinks = {
  setStatusError: (value: unknown) => void;
  setActionError: (value: unknown) => void;
};

export function newRetentionLifetime(): RetentionLifetime {
  return { epoch: 0, mounted: false, mutating: false, reading: null, generation: 0 };
}

// An accepted operation is observed once the status reports that id, or once the
// result slot for its kind is populated. A status read that clears neither means
// the request did not register yet, so polling continues.
export function operationObserved(
  status: { operation?: { id: string }; last_analysis?: unknown; last_run?: unknown },
  accepted: RetentionAcceptedOperation,
) {
  if (status.operation?.id === accepted.id) return true;
  const latest = accepted.kind === "analysis" ? status.last_analysis : status.last_run;
  return latest != null;
}

export function loadRetentionStatus<S>(
  owner: RetentionLifetime,
  fetchStatus: () => Promise<S>,
  accept: (value: S) => void,
  recover: () => void,
  fail: (cause: unknown) => void,
) {
  if (owner.mutating) return Promise.resolve();
  if (owner.reading) return owner.reading;
  const { epoch, generation } = owner;
  const current = () => owner.mounted && owner.epoch === epoch && owner.generation === generation;
  const request = fetchStatus()
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

export async function performRetentionMutation<T>(
  owner: RetentionLifetime,
  request: () => Promise<T>,
  accept: (value: T) => void,
  updates: RetentionErrorSinks & { pending: (value: boolean) => void; clearErrors: () => void },
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
    if (current()) updates.setActionError(cause);
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

export function useRetentionStatusPolling(
  reload: () => Promise<void>,
  active: boolean,
  preparing: boolean,
) {
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

export function useAcceptedOperationRefresh(
  acceptedId: string | null,
  reload: () => Promise<void>,
) {
  useEffect(() => {
    if (!acceptedId) return;
    const timer = setTimeout(() => void reload(), 0);
    return () => clearTimeout(timer);
  }, [acceptedId, reload]);
}

export function useRetentionLifetime(
  owner: { current: RetentionLifetime },
  reload: () => Promise<void>,
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
  }, [owner, reload]);
}

export function useRetentionMutation(
  owner: { current: RetentionLifetime },
  setPending: (value: boolean) => void,
  sinks: RetentionErrorSinks,
) {
  return useCallback(
    <T>(request: () => Promise<T>, accept: (value: T) => void) =>
      performRetentionMutation(owner.current, request, accept, {
        pending: setPending,
        setActionError: sinks.setActionError,
        setStatusError: sinks.setStatusError,
        clearErrors: () => {
          sinks.setStatusError(null);
          sinks.setActionError(null);
        },
      }),
    [owner, sinks],
  );
}

export type RetentionActionSinks = {
  pending: boolean;
  setPending: (value: boolean) => void;
  statusError: unknown;
  actionError: unknown;
  setStatusError: (value: unknown) => void;
  setActionError: (value: unknown) => void;
};

/**
 * Shared pending/error state. The action error and the status error stay
 * separate so a successful read can clear a stale read failure without
 * dismissing an unresolved user-action failure.
 */
export function useRetentionActionSinks(): RetentionActionSinks {
  const [statusError, setStatusError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  return {
    pending,
    setPending,
    statusError,
    setStatusError,
    actionError,
    setActionError,
  };
}
