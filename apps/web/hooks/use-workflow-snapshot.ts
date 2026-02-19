import { useEffect } from "react";
import { fetchWorkflowSnapshot } from "@/lib/api";
import { snapshotToState } from "@/lib/ssr/mapper";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";

export function useWorkflowSnapshot(workflowId: string | null) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!workflowId) return;
    fetchWorkflowSnapshot(workflowId, { cache: "no-store" })
      .then((snapshot) => {
        store.getState().hydrate(snapshotToState(snapshot));
      })
      .catch(() => {
        // Ignore snapshot errors — will retry on WS reconnect.
      });
  }, [workflowId, store, connectionStatus]);
}
