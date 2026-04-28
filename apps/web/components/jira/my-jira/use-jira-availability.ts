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
    // The return-value branch below covers the no-workspace case; calling
    // setAuthed here would trip the no-setState-in-effect lint rule.
    if (!workspaceId) return;
    let cancelled = false;
    async function refresh() {
      try {
        const cfg = await getJiraConfig(workspaceId!);
        if (cancelled) return;
        setAuthed(!!cfg?.hasSecret && !!cfg.lastOk);
      } catch {
        if (!cancelled) setAuthed(false);
      }
    }
    void refresh();
    const id = setInterval(() => void refresh(), JIRA_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId]);
  return workspaceId ? authed : false;
}

// Combined check for showing Jira UI: the workspace toggle is on AND the
// backend reports a configured, healthy connection.
export function useJiraAvailable(workspaceId: string | null | undefined): boolean {
  const ws = workspaceId ?? undefined;
  const { enabled } = useJiraEnabled(ws);
  // Skip the keep-alive probe entirely when Jira is disabled.
  const authed = useJiraAuthed(enabled ? ws : undefined);
  return enabled && authed;
}
