"use client";

import { useEffect, useState } from "react";

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
// probe succeeded.
export function useIntegrationAuthed(
  fetchConfig: () => Promise<IntegrationConfigStatus | null>,
  refreshMs: number = INTEGRATION_STATUS_REFRESH_MS,
): boolean {
  const [authed, setAuthed] = useState(false);
  useEffect(() => {
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
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [fetchConfig, refreshMs]);
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
// the backend reports a configured, healthy connection.
export function useIntegrationAvailable({
  useEnabled,
  fetchConfig,
  refreshMs,
}: IntegrationAvailabilityOptions): boolean {
  const { enabled } = useEnabled();
  const authed = useIntegrationAuthed(fetchConfig, refreshMs);
  return enabled && authed;
}
