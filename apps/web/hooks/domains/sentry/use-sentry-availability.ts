"use client";

import { useCallback, useMemo } from "react";
import { listSentryInstances } from "@/lib/api/domains/sentry-api";
import type { SentryConfig } from "@/lib/types/sentry";
import { INTEGRATION_STATUS_REFRESH_MS } from "../integrations/use-integration-availability";
import { useIntegrationHealth } from "../integrations/use-integration-availability";
import { useSentryEnabled } from "./use-sentry-enabled";

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

// useSentryInstances shares the same workspace-scoped health read and polling
// owner as the other integration availability hooks.
export function useSentryInstances(workspaceId?: string | null): SentryAvailability {
  const { enabled, loaded } = useSentryEnabled(workspaceId);
  const active = loaded && enabled && !!workspaceId;
  const fetchInstances = useCallback(
    () => (workspaceId ? listSentryInstances(workspaceId) : Promise.resolve([])),
    [workspaceId],
  );
  const snapshot = useIntegrationHealth(fetchInstances, {
    provider: "sentry",
    workspaceId,
    refreshMs: INTEGRATION_STATUS_REFRESH_MS,
    active,
  });
  const instances = active ? (snapshot.value ?? []) : [];
  const loading = active && (!snapshot.loaded || snapshot.loading);

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
// the toggle is on and at least one workspace instance is healthy.
export function useSentryAvailable(workspaceId?: string | null): boolean {
  return useSentryInstances(workspaceId).available;
}
