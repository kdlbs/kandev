"use client";

import { useEffect } from "react";
import { listNotificationProviders } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";

export function useNotificationProviders() {
  const providers = useAppStore((state) => state.notificationProviders.items);
  const events = useAppStore((state) => state.notificationProviders.events);
  const appriseAvailable = useAppStore((state) => state.notificationProviders.appriseAvailable);
  const loaded = useAppStore((state) => state.notificationProviders.loaded);
  const loading = useAppStore((state) => state.notificationProviders.loading);
  const setNotificationProviders = useAppStore((state) => state.setNotificationProviders);
  const setNotificationProvidersLoading = useAppStore(
    (state) => state.setNotificationProvidersLoading,
  );

  useEffect(() => {
    if (loaded || loading) return;
    setNotificationProvidersLoading(true);
    listNotificationProviders({ cache: "no-store" })
      .then((response) => {
        setNotificationProviders({
          items: response.providers ?? [],
          events: response.events ?? [],
          appriseAvailable: response.apprise_available ?? false,
          loaded: true,
          loading: false,
        });
      })
      .catch(() => {
        setNotificationProviders({
          items: [],
          events: [],
          appriseAvailable: false,
          loaded: true,
          loading: false,
        });
      })
      .finally(() => {
        setNotificationProvidersLoading(false);
      });
  }, [loaded, loading, setNotificationProviders, setNotificationProvidersLoading]);

  return {
    providers,
    events,
    appriseAvailable,
    loaded,
    loading,
  };
}
