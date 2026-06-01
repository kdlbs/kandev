"use client";

import { useQuery } from "@tanstack/react-query";
import { integrationsQueryOptions } from "@/lib/query/query-options/integrations";

// The backend poller probes credentials roughly every 90s. Refreshing at the
// same cadence keeps the UI no more than ~one cycle stale.
export const INTEGRATION_STATUS_REFRESH_MS = 90_000;

// Shape returned by every integration's `getXConfig` response that this hook
// cares about. Each integration's full config can extend it freely.
export type IntegrationConfigStatus = {
  hasSecret?: boolean;
  lastOk?: boolean;
};

/**
 * Reads the backend-recorded auth health for the install-wide integration.
 * Returns true only when a config exists, has a secret, and the most recent
 * probe succeeded.
 *
 * Pass `active=false` to skip fetching entirely (e.g. while the user toggle
 * is off) — this avoids the polling overhead on disabled integrations.
 *
 * Migration note: previously used useEffect + setInterval + useState.
 * Now delegates to useQuery with refetchInterval: 90_000, which provides
 * the same cadence with deduplication, background refetch, and cache sharing
 * across multiple consumers of the same kind.
 *
 * @param kind      Integration kind string for the cache key, e.g. "jira".
 * @param fetchFn   Integration-specific config fetch function.
 * @param active    Pass false to disable fetching entirely.
 */
export function useIntegrationAuthed(
  kind: string,
  fetchFn: () => Promise<IntegrationConfigStatus | null>,
  active: boolean = true,
): boolean {
  const { data } = useQuery(integrationsQueryOptions.health(kind, fetchFn, active));
  return !!data?.hasSecret && !!data?.lastOk;
}

export type IntegrationAvailabilityOptions = {
  kind: string;
  // Install-wide enabled toggle that has settled. `loaded` gates the
  // probe so we don't waste a fetch on the first render when the toggle is off.
  useEnabled: () => { enabled: boolean; loaded: boolean };
  fetchConfig: () => Promise<IntegrationConfigStatus | null>;
};

/**
 * Combined check for showing an integration's UI: the user toggle is on AND
 * the backend reports a configured, healthy connection.
 *
 * When the toggle is off (or hasn't loaded yet) the auth probe is skipped —
 * disabled integrations don't poll the backend.
 *
 * Migration note: previously called useIntegrationAuthed which internally
 * used useEffect + setInterval. Now both the enabled toggle and the auth
 * probe are read in a single render cycle: `useEnabled` returns synchronously
 * from localStorage (useSyncExternalStore) and `useQuery` returns the cached
 * health value (or undefined while fetching).
 */
export function useIntegrationAvailable({
  kind,
  useEnabled,
  fetchConfig,
}: IntegrationAvailabilityOptions): boolean {
  const { enabled, loaded } = useEnabled();
  const active = loaded && enabled;
  const authed = useIntegrationAuthed(kind, fetchConfig, active);
  return active && authed;
}
