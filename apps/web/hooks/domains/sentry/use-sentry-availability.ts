"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { listSentryInstances } from "@/lib/api/domains/sentry-api";
import type { SentryConfig } from "@/lib/types/sentry";
import { INTEGRATION_STATUS_REFRESH_MS } from "../integrations/use-integration-availability";
import { useSentryEnabled } from "./use-sentry-enabled";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";

// isHealthySentryInstance is the single definition of a usable instance:
// credentials are stored AND the most recent backend probe succeeded.
export function isHealthySentryInstance(instance: SentryConfig): boolean {
  return instance.hasSecret && instance.lastOk;
}

// SentryAvailabilityState distinguishes the shapes the browse surfaces care
// about: still loading, no instances at all, instances but none healthy, one
// healthy (auto-select), or several healthy (must prompt).
export type SentryAvailabilityState = "loading" | "empty" | "unhealthy" | "single" | "multi";

export type SentryAvailability = {
  loading: boolean;
  // instances is every instance in the workspace; healthy is the subset that is
  // authenticated and passing its health probe.
  instances: SentryConfig[];
  healthy: SentryConfig[];
  // available gates whether Sentry entry points render: toggle on AND at least
  // one healthy instance.
  available: boolean;
  state: SentryAvailabilityState;
};

// useSentryInstances polls a workspace's Sentry instances (respecting the
// per-workspace enabled toggle) and derives the availability state the browse
// surfaces and settings banner consume. Fetches are request-versioned so a slow
// response for a previous workspace can't clobber a newer one.
export function useSentryInstances(workspaceId?: string | null): SentryAvailability {
  const { enabled, loaded } = useSentryEnabled();
  const active = loaded && enabled && !!workspaceId;
  const [instances, setInstances] = useState<SentryConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const requestId = useRef(0);

  useEffect(() => {
    if (!active || !workspaceId) {
      // Drop any state carried over from a previous workspace / enabled toggle
      // so a disabled integration shows no stale instances.
      setInstances([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setInstances([]);
    let cancelled = false;
    const refresh = async () => {
      const current = ++requestId.current;
      try {
        const list = await listSentryInstances(workspaceId);
        if (cancelled || current !== requestId.current) return;
        setInstances(list);
      } catch {
        if (cancelled || current !== requestId.current) return;
        setInstances([]);
      } finally {
        if (!cancelled && current === requestId.current) setLoading(false);
      }
    };
    void refresh();
    const id = setInterval(() => void refresh(), INTEGRATION_STATUS_REFRESH_MS);
    const unsubscribe = subscribeIntegrationAvailability(() => void refresh());
    return () => {
      cancelled = true;
      clearInterval(id);
      unsubscribe();
    };
  }, [active, workspaceId]);

  return useMemo(() => {
    const healthy = instances.filter(isHealthySentryInstance);
    const available = active && healthy.length >= 1;
    let state: SentryAvailabilityState;
    if (loading) state = "loading";
    else if (instances.length === 0) state = "empty";
    else if (healthy.length === 0) state = "unhealthy";
    else if (healthy.length === 1) state = "single";
    else state = "multi";
    return { loading, instances, healthy, available, state };
  }, [active, instances, loading]);
}

// useSentryAvailable is the boolean gate that shows/hides Sentry entry points:
// the workspace toggle is on AND at least one instance is healthy.
export function useSentryAvailable(workspaceId?: string | null): boolean {
  return useSentryInstances(workspaceId).available;
}
