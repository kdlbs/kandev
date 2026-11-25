"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getAutomationPluginToolOptions } from "@/lib/api/domains/automation-api";
import type { AutomationPluginToolGroup } from "@/lib/types/automation";

export type AutomationPluginToolsState = {
  groups: AutomationPluginToolGroup[];
  loading: boolean;
  error: boolean;
  reload: () => void;
};

/**
 * Loads the exact plugin tools that may be approved for a private automation.
 * The hook never writes the saved selection: the editor owns that draft and
 * the existing Save action is the approval boundary.
 */
export function useAutomationPluginTools(
  workspaceId: string,
  enabled: boolean,
): AutomationPluginToolsState {
  const [groups, setGroups] = useState<AutomationPluginToolGroup[]>([]);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState(false);
  const [request, setRequest] = useState(0);
  const loadedWorkspace = useRef<string | null>(null);

  const reload = useCallback(() => setRequest((value) => value + 1), []);

  useEffect(() => {
    if (!enabled || !workspaceId) {
      setLoading(false);
      return;
    }
    let active = true;
    if (loadedWorkspace.current !== workspaceId) {
      loadedWorkspace.current = workspaceId;
      setGroups([]);
    }
    setLoading(true);
    setError(false);
    getAutomationPluginToolOptions(workspaceId).then(
      (result) => {
        if (!active) return;
        setGroups(result);
        setLoading(false);
      },
      () => {
        if (!active) return;
        setError(true);
        setLoading(false);
      },
    );
    return () => {
      active = false;
    };
  }, [workspaceId, enabled, request]);

  useEffect(() => {
    if (!enabled || typeof window === "undefined") return;
    const refreshOnFocus = () => reload();
    window.addEventListener("focus", refreshOnFocus);
    return () => window.removeEventListener("focus", refreshOnFocus);
  }, [enabled, reload]);

  return { groups, loading, error, reload };
}
