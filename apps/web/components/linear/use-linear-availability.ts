"use client";

import { useEffect, useState } from "react";
import { getLinearConfig } from "@/lib/api/domains/linear-api";
import { useLinearEnabled } from "./use-linear-enabled";

const LINEAR_STATUS_REFRESH_MS = 90_000;

type AuthState = { workspaceId: string | undefined; authed: boolean };

// Reads the backend-recorded auth health for a workspace. Returns true only
// when a config exists, has a secret, and the most recent probe succeeded.
export function useLinearAuthed(workspaceId: string | undefined): boolean {
  const [state, setState] = useState<AuthState>({ workspaceId: undefined, authed: false });
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    async function refresh() {
      try {
        const cfg = await getLinearConfig(workspaceId!);
        if (cancelled) return;
        setState({ workspaceId, authed: !!cfg?.hasSecret && !!cfg.lastOk });
      } catch {
        if (!cancelled) setState({ workspaceId, authed: false });
      }
    }
    void refresh();
    const id = setInterval(() => void refresh(), LINEAR_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId]);
  return state.workspaceId === workspaceId && state.authed;
}

// Combined check for showing Linear UI: the workspace toggle is on AND the
// backend reports a configured, healthy connection.
export function useLinearAvailable(workspaceId: string | null | undefined): boolean {
  const ws = workspaceId ?? undefined;
  const { enabled, loaded } = useLinearEnabled(ws);
  const authed = useLinearAuthed(enabled && loaded ? ws : undefined);
  return enabled && authed;
}
