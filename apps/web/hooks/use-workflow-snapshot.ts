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
    const setLoading = store.getState().kanban.workflowId !== workflowId;
    if (setLoading) {
      store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: true } }));
    }
    fetchWorkflowSnapshot(workflowId, { cache: "no-store" })
      .then((snapshot) => {
        if (cancelled) return;
        store.getState().hydrate(snapshotToState(snapshot));
      })
      .catch((error) => {
        // Suppress superseded-fetch noise; retry happens on WS reconnect.
        if (cancelled) return;
        console.warn("[useWorkflowSnapshot] failed to load snapshot:", error);
      })
      .finally(() => {
        // Only clear the flag this effect raised; skip when cancelled or when a concurrent caller owns it.
        if (cancelled || !setLoading) return;
        store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: false } }));
      });
    return () => {
      cancelled = true;
    };
  }, [workflowId, store, connectionStatus]);
}
