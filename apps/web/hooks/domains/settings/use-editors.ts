"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import { editorsQueryOptions, userSettingsQueryOptions } from "@/lib/query/query-options/settings";
import { mapUserSettingsQueryData } from "./user-settings-query-data";

export function useEditors() {
  const userSettingsLoaded = useAppStore((state) => state.userSettings.loaded);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const editorsQuery = useQuery(editorsQueryOptions());
  const userSettingsQuery = useQuery({
    ...userSettingsQueryOptions(),
    enabled: !userSettingsLoaded,
  });

  useEffect(() => {
    const client = getWebSocketClient();
    if (client) {
      client.subscribeUser();
    }
  }, []);

  useEffect(() => {
    if (userSettingsLoaded) return;
    const mapped = mapUserSettingsQueryData(userSettingsQuery.data);
    if (!mapped) return;
    setUserSettings(mapped);
  }, [setUserSettings, userSettingsLoaded, userSettingsQuery.data]);

  return {
    editors: editorsQuery.data?.editors ?? [],
    loaded: editorsQuery.isSuccess,
    loading: editorsQuery.isFetching && !editorsQuery.isSuccess,
  };
}
