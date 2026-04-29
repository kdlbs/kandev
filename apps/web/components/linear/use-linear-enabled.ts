"use client";

import { useCallback, useEffect, useState } from "react";

// Workspace-scoped: a workspace can have Linear configured but the user may
// want to silence the integration UI without removing credentials.
const storageKey = (workspaceId: string) => `kandev:linear:enabled:${workspaceId}:v1`;

const SYNC_EVENT = "kandev:linear:enabled-changed";

function readEnabled(workspaceId: string): boolean {
  if (typeof window === "undefined") return true;
  try {
    const raw = window.localStorage.getItem(storageKey(workspaceId));
    if (raw === null) return true;
    return raw !== "false";
  } catch {
    return true;
  }
}

// State tagged with the workspace it was read for. Comparing the tag against
// the currently-requested workspaceId at read time gives us a stale-free
// `loaded` signal without calling setState synchronously inside the effect
// body (which the react-hooks lint rule forbids).
type EnabledState = { workspaceId: string | undefined; enabled: boolean };

export function useLinearEnabled(workspaceId: string | undefined) {
  const [state, setState] = useState<EnabledState>({ workspaceId: undefined, enabled: true });

  useEffect(() => {
    let cancelled = false;
    async function init() {
      if (cancelled) return;
      const next = workspaceId ? readEnabled(workspaceId) : true;
      setState({ workspaceId, enabled: next });
    }
    void init();
    if (!workspaceId) return;
    const onChange = () => {
      const next = readEnabled(workspaceId);
      setState({ workspaceId, enabled: next });
    };
    window.addEventListener("storage", onChange);
    window.addEventListener(SYNC_EVENT, onChange);
    return () => {
      cancelled = true;
      window.removeEventListener("storage", onChange);
      window.removeEventListener(SYNC_EVENT, onChange);
    };
  }, [workspaceId]);

  const setEnabled = useCallback(
    (next: boolean) => {
      if (!workspaceId || typeof window === "undefined") return;
      try {
        window.localStorage.setItem(storageKey(workspaceId), String(next));
        window.dispatchEvent(new Event(SYNC_EVENT));
      } catch {
        // Quota or private mode: state still updates in-memory.
      }
      setState({ workspaceId, enabled: next });
    },
    [workspaceId],
  );

  // `loaded` flips true once the state's workspace tag matches the caller's;
  // this naturally resets when workspaceId changes without us ever writing a
  // synchronous `setLoaded(false)` inside the effect.
  const loaded = state.workspaceId === workspaceId;
  const enabled = loaded ? state.enabled : true;
  return { enabled, setEnabled, loaded };
}
