"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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
  const setAppriseAvailable = useAppStore((state) => state.setAppriseAvailable);
  const [appriseRescanPending, setAppriseRescanPending] = useState(false);
  const [appriseRescanError, setAppriseRescanError] = useState(false);
  const [appriseRescanResult, setAppriseRescanResult] = useState<boolean | null>(null);
  const appriseRescanInFlight = useRef(false);
  const appriseRescanRequestId = useRef(0);

  useEffect(() => {
    return () => {
      // A delayed response must not publish into a later settings-page instance.
      appriseRescanRequestId.current += 1;
    };
  }, []);

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

  const rescanApprise = useCallback(async () => {
    if (loading || appriseRescanInFlight.current) return;
    const requestId = ++appriseRescanRequestId.current;
    appriseRescanInFlight.current = true;
    setAppriseRescanPending(true);
    setAppriseRescanError(false);
    try {
      const response = await listNotificationProviders({ cache: "no-store" });
      if (requestId !== appriseRescanRequestId.current) return;
      const available = response.apprise_available ?? false;
      setAppriseAvailable(available);
      setAppriseRescanResult(available);
    } catch {
      if (requestId !== appriseRescanRequestId.current) return;
      setAppriseRescanError(true);
    } finally {
      if (requestId !== appriseRescanRequestId.current) return;
      appriseRescanInFlight.current = false;
      setAppriseRescanPending(false);
    }
  }, [loading, setAppriseAvailable]);

  return {
    providers,
    events,
    appriseAvailable,
    loaded,
    loading,
    rescanApprise,
    appriseRescanPending,
    appriseRescanError,
    appriseRescanResult,
  };
}
