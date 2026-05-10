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
        // Surface the failure so users see it rather than an indefinite empty
        // list. Retry happens on WS reconnect.
        console.warn("[useWorkflowSnapshot] failed to load snapshot:", error);
      })
      .finally(() => {
        // Only clear what this effect set. Skipping when cancelled avoids
        // collapsing the new effect's skeleton; skipping when !setLoading
        // avoids stomping on a flag a concurrent caller (e.g. workspace
        // switch) raised independently.
        if (cancelled || !setLoading) return;
        store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: false } }));
      });
    return () => {
      cancelled = true;
    };
  }, [workflowId, store, connectionStatus]);
}
