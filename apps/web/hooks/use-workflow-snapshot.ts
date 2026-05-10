import { useEffect } from "react";
import { fetchWorkflowSnapshot } from "@/lib/api";
import { snapshotToState } from "@/lib/ssr/mapper";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";

export function useWorkflowSnapshot(workflowId: string | null) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!workflowId) return;
    let cancelled = false;
    const alreadyHydrated = store.getState().kanban.workflowId === workflowId;
    if (!alreadyHydrated) {
      store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: true } }));
    }
    fetchWorkflowSnapshot(workflowId, { cache: "no-store" })
      .then((snapshot) => {
        if (cancelled) return;
        store.getState().hydrate(snapshotToState(snapshot));
      })
      .catch((error) => {
        // Surface the failure so users see it rather than an indefinite empty
        // list. Retry happens on WS reconnect.
        console.warn("[useWorkflowSnapshot] failed to load snapshot:", error);
      })
      .finally(() => {
        // Skip when the effect was superseded — otherwise an in-flight fetch
        // for an old workflowId would clear the loading flag the new effect
        // just set, briefly flashing the empty-state UI. The next effect's
        // finally settles isLoading for the active workflow.
        if (cancelled) return;
        store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: false } }));
      });
    return () => {
      cancelled = true;
    };
  }, [workflowId, store, connectionStatus]);
}
