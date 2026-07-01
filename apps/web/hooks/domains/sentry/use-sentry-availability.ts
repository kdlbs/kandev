"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { sentryInstancesQueryOptions } from "@/lib/query/query-options/sentry";
import type { SentryConfig } from "@/lib/types/sentry";
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

// useSentryInstances reads a workspace's Sentry instances from Query
// (respecting the per-workspace enabled toggle) and derives the availability
// state the browse surfaces and settings banner consume.
export function useSentryInstances(workspaceId?: string | null): SentryAvailability {
  const { enabled, loaded } = useSentryEnabled();
  const active = loaded && enabled && !!workspaceId;
  const query = useQuery(sentryInstancesQueryOptions(active ? workspaceId : null));

  return useMemo(() => {
    const instances = active ? (query.data ?? []) : [];
    const healthy = instances.filter(isHealthySentryInstance);
    const loading = active && (query.isPending || (query.isFetching && !query.data));
    const available = active && healthy.length >= 1;
    let state: SentryAvailabilityState;
    if (loading) state = "loading";
    else if (instances.length === 0) state = "empty";
    else if (healthy.length === 0) state = "unhealthy";
    else if (healthy.length === 1) state = "single";
    else state = "multi";
    return { loading, instances, healthy, available, state };
  }, [active, query.data, query.isFetching, query.isPending]);
}

export function useSentryAuthed(workspaceId?: string | null): boolean {
  return useSentryInstances(workspaceId).instances.some((instance) => instance.hasSecret);
}

// useSentryAvailable is the boolean gate that shows/hides Sentry entry points:
// the workspace toggle is on AND at least one instance is healthy.
export function useSentryAvailable(workspaceId?: string | null): boolean {
  return useSentryInstances(workspaceId).available;
}
