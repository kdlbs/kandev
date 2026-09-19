"use client";

import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import {
  isValidRemoteExecutorStatusRequest,
  remoteExecutorStatusResource,
  remoteExecutorStatusScope,
  type RemoteExecutorStatusRequest,
} from "./remote-executor-status-resource";

export function useRemoteExecutorStatus(request: RemoteExecutorStatusRequest, enabled: boolean) {
  const valid = enabled && isValidRemoteExecutorStatusRequest(request);
  const scope = valid ? remoteExecutorStatusScope(request) : "";
  const stableRequest = useMemo(
    () => request,
    [request.executorId, request.executorType, request.sessionId, request.taskId],
  );
  const subscribe = useCallback(
    (listener: () => void) => remoteExecutorStatusResource.subscribe(scope, listener),
    [scope],
  );
  const getSnapshot = useCallback(() => remoteExecutorStatusResource.getSnapshot(scope), [scope]);
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  useEffect(() => {
    if (!valid) return;
    void remoteExecutorStatusResource.load(stableRequest);
  }, [scope, stableRequest, valid]);

  const refresh = useCallback(() => {
    if (!valid) return Promise.resolve(null);
    return Promise.resolve(remoteExecutorStatusResource.load(stableRequest, true));
  }, [stableRequest, valid]);

  return { ...snapshot, refresh };
}
