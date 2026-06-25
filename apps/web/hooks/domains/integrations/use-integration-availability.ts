"use client";

import { useQuery } from "@tanstack/react-query";

// The backend poller probes credentials roughly every 90s. Refreshing at the
// same cadence keeps the UI no more than ~one cycle stale.
export const INTEGRATION_STATUS_REFRESH_MS = 90_000;

// Shape returned by every integration's `getXConfig` response that this hook
// cares about. Each integration's full config can extend it freely.
export type IntegrationConfigStatus = {
  hasSecret?: boolean;
  lastOk?: boolean;
};

export type IntegrationAuthOptions = {
  active?: boolean;
  fetchConfig: () => Promise<IntegrationConfigStatus | null>;
  queryKey: readonly unknown[];
  refreshMs?: number;
};

// Reads the backend-recorded auth health for the install-wide integration.
// Returns true only when a config exists, has a secret, and the most recent
// probe succeeded. Pass `active=false` to skip fetching entirely (e.g. while
// the user toggle is off) — this avoids the polling overhead on disabled
// integrations.
export function useIntegrationAuthed({
  active = true,
  fetchConfig,
  queryKey,
  refreshMs = INTEGRATION_STATUS_REFRESH_MS,
}: IntegrationAuthOptions): boolean {
  const query = useQuery({
    queryKey,
    queryFn: fetchConfig,
    enabled: active,
    refetchInterval: active ? refreshMs : false,
    retry: false,
  });
  return active && !!query.data?.hasSecret && !!query.data.lastOk;
}

export type IntegrationAvailabilityOptions = {
  // Install-wide enabled toggle that has settled. `loaded` gates the
  // probe so we don't waste a fetch on the first render when the toggle is
  // off.
  useEnabled: () => { enabled: boolean; loaded: boolean };
  fetchConfig: () => Promise<IntegrationConfigStatus | null>;
  queryKey: readonly unknown[];
  refreshMs?: number;
};

// Combined check for showing an integration's UI: the user toggle is on AND
// the backend reports a configured, healthy connection. When the toggle is
// off (or hasn't loaded yet) the auth probe is skipped — disabled
// integrations don't poll the backend.
export function useIntegrationAvailable({
  useEnabled,
  fetchConfig,
  queryKey,
  refreshMs,
}: IntegrationAvailabilityOptions): boolean {
  const { enabled, loaded } = useEnabled();
  const active = loaded && enabled;
  const authed = useIntegrationAuthed({ active, fetchConfig, queryKey, refreshMs });
  return active && authed;
}
