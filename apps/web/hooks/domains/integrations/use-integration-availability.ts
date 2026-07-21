"use client";

import { useEffect, useState } from "react";
import {
  invalidateIntegrationAvailability,
  subscribeIntegrationAvailability,
} from "@/lib/integrations/integration-availability-events";

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

// Reads the backend-recorded auth health for the install-wide integration.
// Returns true only when a config exists, has a secret, and the most recent
// probe succeeded. Pass `active=false` to skip fetching entirely (e.g. while
// the user toggle is off) — this avoids the polling overhead on disabled
// integrations.
export function useIntegrationAuthed(
  fetchConfig: () => Promise<IntegrationConfigStatus | null>,
  refreshMs: number = INTEGRATION_STATUS_REFRESH_MS,
  active: boolean = true,
): boolean {
  const [authed, setAuthed] = useState(false);
  useEffect(() => {
    if (!active) {
      setAuthed(false);
      return;
    }
    // Drop any auth state carried over from a previous `fetchConfig` before the
    // new probe resolves. `fetchConfig` is keyed by workspace for per-workspace
    // integrations, so a workspace switch must not keep showing the previous
    // workspace's "authed" result during the in-flight recheck.
    setAuthed(false);
    let cancelled = false;
    // Monotonic request id: if a slow earlier probe finishes after a newer
    // one we ignore it, otherwise an old "auth ok" could clobber a fresh
    // "auth failed" (or vice versa) and the UI would flap until the next
    // tick.
    let requestId = 0;
    async function refresh() {
      const current = ++requestId;
      try {
        const cfg = await fetchConfig();
        if (cancelled || current !== requestId) return;
        setAuthed(!!cfg?.hasSecret && !!cfg.lastOk);
      } catch {
        if (cancelled || current !== requestId) return;
        setAuthed(false);
      }
    }
    void refresh();
    const id = setInterval(() => void refresh(), refreshMs);
    const unsubscribe = subscribeIntegrationAvailability(() => void refresh());
    return () => {
      cancelled = true;
      clearInterval(id);
      unsubscribe();
    };
  }, [active, fetchConfig, refreshMs]);
  return authed;
}

export type IntegrationAvailabilityOptions = {
  // Install-wide enabled toggle that has settled. `loaded` gates the
  // probe so we don't waste a fetch on the first render when the toggle is
  // off.
  useEnabled: () => { enabled: boolean; loaded: boolean };
  fetchConfig: () => Promise<IntegrationConfigStatus | null>;
  refreshMs?: number;
};

// Combined check for showing an integration's UI: the user toggle is on AND
// the backend reports a configured, healthy connection. When the toggle is
// off (or hasn't loaded yet) the auth probe is skipped — disabled
// integrations don't poll the backend.
export function useIntegrationAvailable({
  useEnabled,
  fetchConfig,
  refreshMs,
}: IntegrationAvailabilityOptions): boolean {
  const { enabled, loaded } = useEnabled();
  const active = loaded && enabled;
  const authed = useIntegrationAuthed(fetchConfig, refreshMs, active);
  return active && authed;
}
