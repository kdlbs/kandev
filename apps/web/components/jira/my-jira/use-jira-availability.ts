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
export function useJiraAuthed(workspaceId: string | undefined): boolean {
  const [authed, setAuthed] = useState(false);
  useEffect(() => {
    let cancelled = false;
    async function refresh() {
      // Reset before the first probe so the previous workspace's auth state
      // can't briefly leak into the new one before the first probe lands.
      if (!cancelled) setAuthed(false);
      if (!workspaceId) return;
      try {
        const cfg = await getJiraConfig(workspaceId);
        if (cancelled) return;
        setAuthed(!!cfg?.hasSecret && !!cfg.lastOk);
      } catch {
        if (!cancelled) setAuthed(false);
      }
    }
    void refresh();
    if (!workspaceId)
      return () => {
        cancelled = true;
      };
    const id = setInterval(() => void refresh(), JIRA_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId]);
  return authed;
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
