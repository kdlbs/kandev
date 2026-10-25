import { useCallback, useSyncExternalStore } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  environmentReadIdentity,
  getSessionReadScope,
  subscribeSessionReadScope,
  type SessionReadOptions,
  type SessionReadSnapshot,
} from "@/lib/state/session-read-coordinator";

const EMPTY: SessionReadSnapshot<never> = { data: undefined, loading: false, error: null };
const subscribeEmpty = () => () => {};
const getEmpty = () => EMPTY;

export function useSessionReadScope() {
  const store = useAppStoreApi();
  const subscribe = useCallback(
    (listener: () => void) => subscribeSessionReadScope(store, listener),
    [store],
  );
  const getSnapshot = useCallback(() => getSessionReadScope(store), [store]);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

export function useSessionRead<T>(
  options: (Omit<SessionReadOptions<T>, "isCurrent"> & { sessionId?: string }) | null,
) {
  const store = useAppStoreApi();
  const scope = useSessionReadScope();
  const environmentId = options?.environmentId ?? "";
  const sessionId = options?.sessionId;
  const restoration = useAppStore((state) => environmentReadIdentity(state, environmentId));
  const read = options
    ? scope.get({
        ...options,
        key: JSON.stringify([options.key, restoration]),
        isCurrent: () =>
          environmentReadIdentity(store.getState(), environmentId) === restoration &&
          (!sessionId ||
            (store.getState().environmentIdBySessionId[sessionId] ?? sessionId) === environmentId),
      })
    : null;
  const snapshot = useSyncExternalStore(
    read?.subscribe ?? subscribeEmpty,
    read?.getSnapshot ?? getEmpty,
    read?.getSnapshot ?? getEmpty,
  );
  return { read, snapshot, scope };
}
