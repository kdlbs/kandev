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

export function useLinearEnabled(workspaceId: string | undefined) {
  const [enabled, setEnabledState] = useState(true);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function init() {
      if (cancelled) return;
      if (workspaceId) setEnabledState(readEnabled(workspaceId));
      setLoaded(true);
    }
    void init();
    if (!workspaceId) return;
    const onChange = () => setEnabledState(readEnabled(workspaceId));
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
      setEnabledState(next);
    },
    [workspaceId],
  );

  return { enabled, setEnabled, loaded };
}
