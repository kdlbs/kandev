"use client";

import { useEffect } from "react";
import { listEditors } from "@/lib/api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import { useEnsureUserSettings } from "@/hooks/use-ensure-user-settings";

export function useEditors() {
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
    if (loaded || loading) return;
    setEditorsLoading(true);
    listEditors({ cache: "no-store" })
      .then((response) => {
        setEditors(response.editors ?? []);
      })
      .catch(() => {
        setEditors([]);
      })
      .finally(() => {
        setEditorsLoading(false);
      });
  }, [loaded, loading, setEditors, setEditorsLoading]);

  return { editors, loaded, loading };
}
