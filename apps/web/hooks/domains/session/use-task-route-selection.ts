import { useCallback, useLayoutEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { storeReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  admitExplicitTaskNavigationSelection,
  type TaskNavigationSelectionOwner,
} from "@/lib/state/task-navigation-selection";

type DeferredRoute = { tasks: AppState["tasks"]; scope: string };

/** A pending workflow delays an explicit route; only committed selection supersedes it. */
export function useTaskRouteSelection(
  requestedSessionId: string | undefined,
  owner: TaskNavigationSelectionOwner | undefined,
) {
  const store = useAppStoreApi();
  const workflow = useAppStore((state) =>
    requestedSessionId && owner ? state.workflowSessionFocus : null,
  );
  const pending = useRef<DeferredRoute | null>(null);
  useLayoutEffect(() => {
    pending.current = null;
    return () => {
      pending.current = null;
    };
  }, [store, requestedSessionId, owner]);

  useLayoutEffect(() => {
    const candidate = pending.current;
    if (!candidate) return;
    pending.current = null;
    if (candidate.scope !== storeReadScopeIdentity(store)) return;
    const result = admitExplicitTaskNavigationSelection(
      store,
      {
        tasks: candidate.tasks,
        taskSessionsByTask: store.getState().taskSessionsByTask,
      },
      requestedSessionId,
      owner,
    );
    if (result === "deferred") pending.current = candidate;
  }, [store, requestedSessionId, owner, workflow]);

  return useCallback(
    (incoming: Partial<AppState>) => {
      if (!incoming.tasks) return;
      const result = admitExplicitTaskNavigationSelection(
        store,
        incoming,
        requestedSessionId,
        owner,
      );
      pending.current =
        result === "deferred"
          ? { tasks: incoming.tasks, scope: storeReadScopeIdentity(store) }
          : null;
    },
    [store, requestedSessionId, owner],
  );
}
