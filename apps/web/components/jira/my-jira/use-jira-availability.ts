"use client";

import { useEffect, useState } from "react";
import { getJiraConfig } from "@/lib/api/domains/jira-api";
import { useJiraEnabled } from "./use-jira-enabled";

// The backend poller probes Jira credentials roughly every 90s. Refreshing
// at the same cadence keeps the UI no more than ~one cycle stale.
const JIRA_STATUS_REFRESH_MS = 90_000;

// Reads the backend-recorded auth health for a workspace. Returns true only
// when a config exists, has a secret, and the most recent probe succeeded.
// Pass `undefined` to short-circuit (no fetch, no interval).
// State is tagged with the workspace it was probed for so a workspace switch
// invalidates stale results synchronously at read time — no reset needed,
// which keeps the 90s poll tick from flickering the UI between request
// dispatch and response.
type AuthState = { workspaceId: string | undefined; authed: boolean };

export function useJiraAuthed(workspaceId: string | undefined): boolean {
  const [state, setState] = useState<AuthState>({ workspaceId: undefined, authed: false });
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    async function refresh() {
      try {
        const cfg = await getJiraConfig(workspaceId!);
        if (cancelled) return;
        setState({ workspaceId, authed: !!cfg?.hasSecret && !!cfg.lastOk });
      } catch {
        if (!cancelled) setState({ workspaceId, authed: false });
      }
    }
    void refresh();
    const id = setInterval(() => void refresh(), JIRA_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId]);
  return state.workspaceId === workspaceId && state.authed;
}

// Combined check for showing Jira UI: the workspace toggle is on AND the
// backend reports a configured, healthy connection.
export function useJiraAvailable(workspaceId: string | null | undefined): boolean {
  const ws = workspaceId ?? undefined;
  // `loaded` flips true after the localStorage read settles; gating the probe
  // on it avoids a wasted fetch on the first render when the toggle is off.
  const { enabled, loaded } = useJiraEnabled(ws);
  const authed = useJiraAuthed(enabled && loaded ? ws : undefined);
  return enabled && authed;
}
