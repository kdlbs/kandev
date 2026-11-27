"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  invalidateIntegrationAvailability,
  subscribeIntegrationAvailability,
} from "@/lib/integrations/integration-availability-events";
import {
  getIntegrationHealthResourceScope,
  integrationHealthAuthScopeIdentity,
  type IntegrationHealthProvider,
  type IntegrationHealthSnapshot,
} from "./integration-health-resource";

export { invalidateIntegrationAvailability };

// The backend poller probes credentials roughly every 90s. Refreshing at the
// same cadence keeps the UI no more than ~one cycle stale.
export const INTEGRATION_STATUS_REFRESH_MS = 90_000;

// Shape returned by every integration's `getXConfig` response that this hook
// cares about. Each integration's full config can extend it freely.
export type IntegrationConfigStatus = {
  hasSecret?: boolean;
  lastOk?: boolean;
};

const EMPTY_SNAPSHOT: IntegrationHealthSnapshot<never> = {
  value: null,
  loading: false,
  loaded: false,
  error: null,
  lastUpdatedAt: null,
};

export type IntegrationAvailabilityScope = {
  provider: IntegrationHealthProvider;
  workspaceId?: string | null;
  refreshMs?: number;
  active?: boolean;
};

export function useIntegrationHealth<T>(
  fetchConfig: () => Promise<T>,
  {
    provider,
    workspaceId,
    refreshMs = INTEGRATION_STATUS_REFRESH_MS,
    active = true,
  }: IntegrationAvailabilityScope,
): IntegrationHealthSnapshot<T> {
  const store = useAppStoreApi();
  const identity = useAppStore(integrationHealthAuthScopeIdentity);
  const scope = useMemo(() => getIntegrationHealthResourceScope(store), [identity, store]);
  const key = active ? scope.key(provider, workspaceId) : null;
  const subscribe = useCallback(
    (listener: () => void) =>
      key ? scope.subscribe(key, listener, fetchConfig, refreshMs) : () => {},
    [fetchConfig, key, refreshMs, scope],
  );
  const getSnapshot = useCallback(
    () => (key ? scope.getSnapshot<T>(key) : EMPTY_SNAPSHOT),
    [key, scope],
  );
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

// Reads backend-recorded auth health for the integration. A successful null
// response is a settled no-config result and does not trigger more reads when
// another consumer mounts.
export function useIntegrationAuthed(
  fetchConfig: () => Promise<IntegrationConfigStatus | null>,
  scope: IntegrationAvailabilityScope,
): boolean {
  const snapshot = useIntegrationHealth(fetchConfig, scope);
  return (scope.active ?? true) && !!snapshot.value?.hasSecret && !!snapshot.value.lastOk;
}

export type IntegrationAvailabilityOptions = {
  provider: IntegrationHealthProvider;
  workspaceId?: string | null;
  // The workspace's enabled toggle, already read by the caller (it owns the
  // workspace id its fetchConfig is keyed by, so it owns the toggle read too).
  enabledState: { enabled: boolean; loaded: boolean };
  fetchConfig: () => Promise<IntegrationConfigStatus | null>;
  refreshMs?: number;
};

// Combined check for showing an integration's UI: the workspace's toggle is on
// AND the backend reports a configured, healthy connection. Disabled
// integrations are not subscribed and do not probe the backend.
export function useIntegrationAvailable({
  enabledState,
  fetchConfig,
  provider,
  workspaceId,
  refreshMs,
}: IntegrationAvailabilityOptions): boolean {
  const { enabled, loaded } = enabledState;
  const active = loaded && enabled;
  const authed = useIntegrationAuthed(fetchConfig, {
    provider,
    workspaceId,
    refreshMs,
    active,
  });
  return active && authed;
}

// Kept as a direct export for callers that need to subscribe to credential
// invalidation without owning the health resource.
export { subscribeIntegrationAvailability };
