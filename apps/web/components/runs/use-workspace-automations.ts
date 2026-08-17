"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listAutomations } from "@/lib/api/domains/automation-api";
import type { Automation } from "@/lib/types/automation";
import { t } from "@/lib/i18n";

const EMPTY_AUTOMATIONS: Automation[] = [];

/**
 * Automations for the runs list, fetched here rather than read from the shared
 * `automations` store slot.
 *
 * That slot is global and the settings pages write to it, so reading it here
 * would show whichever workspace's automations were fetched last. This list
 * follows the active workspace and nothing else.
 *
 * The workspace is stored alongside the rows for the same reason it is in
 * `useWorkspaceRuns`: clearing inside the fetch effect is too late, because the
 * effect is passive and React commits one render with the previous workspace's
 * rows still on screen before it runs. Keeping the owner with the data lets the
 * hook mask them during render, so another workspace's automations never appear
 * under this one — not even for a frame.
 */
type LoadedAutomations = {
  workspaceId: string | undefined;
  automations: Automation[];
};

const NOTHING_LOADED: LoadedAutomations = {
  workspaceId: undefined,
  automations: EMPTY_AUTOMATIONS,
};

export function useWorkspaceAutomations(workspaceId: string | undefined) {
  const [loaded, setLoaded] = useState<LoadedAutomations>(NOTHING_LOADED);
  const [loading, setLoading] = useState(Boolean(workspaceId));
  const [error, setError] = useState<string | null>(null);
  // Ordered by issue, not by arrival: a slow first load must not overwrite a
  // refresh the user asked for afterwards.
  const requestRef = useRef(0);

  const refresh = useCallback(() => {
    // Claimed even with no workspace, so a response already in flight for the
    // previous workspace cannot land and repopulate a list that should be empty.
    const requestId = ++requestRef.current;

    if (!workspaceId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    listAutomations(workspaceId)
      .then((result) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ workspaceId, automations: result ?? EMPTY_AUTOMATIONS });
        setError(null);
      })
      .catch((err: unknown) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ workspaceId, automations: EMPTY_AUTOMATIONS });
        setError(err instanceof Error ? err.message : t("automations:failedToLoadAutomations"));
      })
      .finally(() => {
        if (requestRef.current !== requestId) return;
        setLoading(false);
      });
  }, [workspaceId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const switching = loaded.workspaceId !== workspaceId;
  const automations = switching ? EMPTY_AUTOMATIONS : loaded.automations;
  const staleError = switching ? null : error;
  // Derived for the same reason the rows are: the fetch is issued from a
  // passive effect, so on a switch there is a render where nothing has loaded
  // and nothing has been requested yet. Reporting "not loading" there flashes
  // the empty state before the request even starts.
  const pending = loading || (switching && Boolean(workspaceId));

  return { automations, loading: pending, error: staleError, refresh };
}
