"use client";

import { useCallback, useEffect, useState } from "react";

// useIntegrationEnabled is the workspace-scoped on/off toggle every
// third-party integration UI (jira, linear, future) needs: a localStorage-
// backed boolean that defaults to true, syncs across tabs via the `storage`
// event, and within a tab via a custom event the integration provides.
//
// State is tagged with the workspace it was read for. Comparing the tag at
// read time gives us a stale-free `loaded` signal without ever having to call
// setLoaded(false) inside the effect (which would be a sync-set-state inside
// useEffect that the react-hooks lint rule forbids).
type EnabledState = { workspaceId: string | undefined; enabled: boolean };

export type IntegrationEnabledOptions = {
  // localStorage key for this workspace, e.g. `kandev:jira:enabled:ws-1:v1`.
  storageKey: (workspaceId: string) => string;
  // Custom event fired on the window when the toggle changes within a tab.
  // The `storage` event only fires across tabs, so each integration needs
  // its own intra-tab signal — different integrations get different events
  // so toggling one doesn't re-render every consumer of the others.
  syncEvent: string;
};

function readEnabled(
  workspaceId: string,
  storageKey: IntegrationEnabledOptions["storageKey"],
): boolean {
  if (typeof window === "undefined") return true;
  try {
    const raw = window.localStorage.getItem(storageKey(workspaceId));
    if (raw === null) return true;
    return raw !== "false";
  } catch {
    return true;
  }
}

export function useIntegrationEnabled(
  workspaceId: string | undefined,
  { storageKey, syncEvent }: IntegrationEnabledOptions,
) {
  const [state, setState] = useState<EnabledState>({ workspaceId: undefined, enabled: true });

  useEffect(() => {
    let cancelled = false;
    function init() {
      if (cancelled) return;
      const next = workspaceId ? readEnabled(workspaceId, storageKey) : true;
      setState({ workspaceId, enabled: next });
    }
    init();
    if (!workspaceId) return;
    const onChange = () => {
      const next = readEnabled(workspaceId, storageKey);
      setState({ workspaceId, enabled: next });
    };
    window.addEventListener("storage", onChange);
    window.addEventListener(syncEvent, onChange);
    return () => {
      cancelled = true;
      window.removeEventListener("storage", onChange);
      window.removeEventListener(syncEvent, onChange);
    };
  }, [workspaceId, storageKey, syncEvent]);

  const setEnabled = useCallback(
    (next: boolean) => {
      if (!workspaceId || typeof window === "undefined") return;
      try {
        window.localStorage.setItem(storageKey(workspaceId), String(next));
        window.dispatchEvent(new Event(syncEvent));
      } catch {
        // Quota or private mode: state still updates in-memory.
      }
      setState({ workspaceId, enabled: next });
    },
    [workspaceId, storageKey, syncEvent],
  );

  // `loaded` flips true once the state's workspace tag matches the caller's;
  // this naturally resets when workspaceId changes without us ever writing a
  // synchronous `setLoaded(false)` inside the effect.
  const loaded = state.workspaceId === workspaceId;
  const enabled = loaded ? state.enabled : true;
  return { enabled, setEnabled, loaded };
}
