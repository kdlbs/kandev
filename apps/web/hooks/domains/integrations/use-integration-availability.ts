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

// Reads the backend-recorded auth health for a workspace. Returns true only
// when a config exists, has a secret, and the most recent probe succeeded.
// Pass `undefined` to short-circuit (no fetch, no interval).
//
// State is tagged with the workspace it was probed for so a workspace switch
// invalidates stale results synchronously at read time — no reset needed,
// which keeps the 90s poll tick from flickering the UI between request
// dispatch and response.
type AuthState = { workspaceId: string | undefined; authed: boolean };

export function useIntegrationAuthed(
  workspaceId: string | undefined,
  fetchConfig: (workspaceId: string) => Promise<IntegrationConfigStatus | null>,
  refreshMs: number = INTEGRATION_STATUS_REFRESH_MS,
): boolean {
  const [state, setState] = useState<AuthState>({ workspaceId: undefined, authed: false });
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    // Monotonic request id: if a slow earlier probe finishes after a newer
    // one we ignore it, otherwise an old "auth ok" could clobber a fresh
    // "auth failed" (or vice versa) and the UI would flap until the next
    // tick.
    let requestId = 0;
    async function refresh() {
      const current = ++requestId;
      try {
        const cfg = await fetchConfig(workspaceId!);
        if (cancelled || current !== requestId) return;
        setState({ workspaceId, authed: !!cfg?.hasSecret && !!cfg.lastOk });
      } catch {
        if (cancelled || current !== requestId) return;
        setState({ workspaceId, authed: false });
      }
    }
    void refresh();
    const id = setInterval(() => void refresh(), refreshMs);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId, fetchConfig, refreshMs]);
  return state.workspaceId === workspaceId && state.authed;
}

export type IntegrationAvailabilityOptions = {
  // Workspace-scoped enabled toggle that has settled. `loaded` gates the
  // probe so we don't waste a fetch on the first render when the toggle is
  // off.
  useEnabled: (workspaceId: string | undefined) => { enabled: boolean; loaded: boolean };
  fetchConfig: (workspaceId: string) => Promise<IntegrationConfigStatus | null>;
  refreshMs?: number;
};

// Combined check for showing an integration's UI: the workspace toggle is on
// AND the backend reports a configured, healthy connection.
export function useIntegrationAvailable(
  workspaceId: string | null | undefined,
  { useEnabled, fetchConfig, refreshMs }: IntegrationAvailabilityOptions,
): boolean {
  const ws = workspaceId ?? undefined;
  const { enabled, loaded } = useEnabled(ws);
  const authed = useIntegrationAuthed(enabled && loaded ? ws : undefined, fetchConfig, refreshMs);
  return enabled && authed;
}
