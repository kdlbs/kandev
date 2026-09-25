"use client";

import { useEffect } from "react";
import { listEditors } from "@/lib/api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useEnsureUserSettings } from "@/hooks/use-ensure-user-settings";

export function useEditors() {
  const store = useAppStoreApi();
  const folderOpeningAvailable = useAppStore((state) => state.editors.folderOpeningAvailable);
  const editors = useAppStore((state) => state.editors.items);
  const loaded = useAppStore((state) => state.editors.loaded);
  const loading = useAppStore((state) => state.editors.loading);
  const setEditors = useAppStore((state) => state.setEditors);
  const setEditorsLoading = useAppStore((state) => state.setEditorsLoading);
  useEnsureUserSettings();

  useEffect(() => {
    const client = getWebSocketClient();
    if (client) {
      client.subscribeUser();
    }
  }, []);

  useEffect(() => {
    const current = store.getState().editors;
    if ((current.loaded && current.folderOpeningAvailable !== undefined) || current.loading) return;
    setEditorsLoading(true);
    listEditors({ cache: "no-store" })
      .catch(async () => {
        // Keep one shared discovery in flight and retry transient failures once.
        await new Promise((resolve) => setTimeout(resolve, 1000));
        return listEditors({ cache: "no-store" });
      })
      .then((response) => {
        setEditors(response.editors ?? [], response.folder_opening_available === true);
      })
      .catch(() => {
        setEditors([], false);
      })
      .finally(() => {
        setEditorsLoading(false);
      });
  }, [store, loaded, loading, folderOpeningAvailable, setEditors, setEditorsLoading]);

  return { editors, loaded, loading, folderOpeningAvailable: folderOpeningAvailable === true };
}
