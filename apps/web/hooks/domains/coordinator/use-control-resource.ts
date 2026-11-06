import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { NativeControlContractError } from "@/lib/api/domains/control-native-api";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";

export type ControlReadStatus = "loading" | "ready" | "unsupported" | "denied" | "error";
export type ControlRead<T> = {
  status: ControlReadStatus;
  data?: T;
  receivedAt?: string;
  refreshing?: boolean;
  stale?: boolean;
};

/** Route-owned reads follow the existing control snapshot. No timer or provider refresh. */
export function useControlResource<T>(
  read: (signal: AbortSignal) => Promise<T>,
  revision: object | undefined,
  reconcile?: (previous: T | undefined, next: T) => T,
): ControlRead<T> {
  const identity = useAppStore(stateReadScopeIdentity);
  const [result, setResult] = useState<
    ControlRead<T> & { identity: string; read: typeof read; revision: typeof revision }
  >();
  useEffect(() => {
    if (!revision) return;
    const request = new AbortController();
    const publish = (update: (previous?: ControlRead<T>) => ControlRead<T>) => {
      if (request.signal.aborted) return;
      setResult((previous) => ({
        ...update(previous?.identity === identity && previous.read === read ? previous : undefined),
        identity,
        read,
        revision,
      }));
    };
    void read(request.signal).then(
      (data) =>
        publish((previous) => ({
          status: "ready",
          data: reconcile ? reconcile(previous?.data, data) : data,
          receivedAt: new Date().toISOString(),
        })),
      (error: unknown) =>
        publish((previous) => {
          const status = controlReadFailure(error);
          return status === "error" && previous?.data !== undefined
            ? { ...previous, status, stale: true, refreshing: false }
            : { status };
        }),
    );
    return () => request.abort();
  }, [identity, read, revision, reconcile]);
  if (!revision || result?.identity !== identity || result.read !== read)
    return { status: "loading" };
  return result.revision === revision ? result : { ...result, refreshing: true };
}

/** A transient row failure retains only the same authorized row's dated snapshot. */
export function retainControlRows<T extends { id: string; status: ControlReadStatus }>(
  previous: T[] | undefined,
  next: T[],
): Array<T & { receivedAt?: string; stale?: boolean }> {
  const at = new Date().toISOString();
  return next.map((row) => {
    const old = previous?.find((item) => item.id === row.id) as
      | (T & { receivedAt?: string })
      | undefined;
    if (row.status === "error" && old?.receivedAt)
      return { ...old, status: row.status, stale: true };
    return { ...row, receivedAt: row.status === "ready" ? at : undefined };
  });
}

export function controlReadFailure(error: unknown): ControlReadStatus {
  if (error instanceof NativeControlContractError) return "unsupported";
  if (error instanceof ApiError) {
    if (error.status === 401 || error.status === 403) return "denied";
    if (error.status === 404 || error.status === 501) return "unsupported";
  }
  return "error";
}
