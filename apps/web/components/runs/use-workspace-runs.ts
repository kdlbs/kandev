"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listWorkspaceAutomationRuns } from "@/lib/api/domains/automation-api";
import type { WorkspaceAutomationRun } from "@/lib/types/automation";
import { t } from "@/lib/i18n";

/**
 * The feed is a reading surface, not an audit trail — a couple of hundred rows
 * is far more than anyone scrolls, and the filters narrow what is already here
 * rather than re-querying.
 */
export const WORKSPACE_RUNS_LIMIT = 200;

const EMPTY_RUNS: WorkspaceAutomationRun[] = [];

/**
 * Rows are stored with the workspace they came from rather than on their own.
 *
 * Clearing them inside the fetch effect is too late: the effect is passive, so
 * React commits one render with the previous workspace's rows still on screen
 * before it runs. Keeping the owner alongside the data lets the hook mask them
 * during render, so another workspace's activity is never shown under this
 * workspace's name — not even for a frame.
 */
type LoadedRuns = {
  workspaceId: string | undefined;
  runs: WorkspaceAutomationRun[];
};

const NOTHING_LOADED: LoadedRuns = { workspaceId: undefined, runs: EMPTY_RUNS };

export function useWorkspaceRuns(workspaceId: string | undefined) {
  const [loaded, setLoaded] = useState<LoadedRuns>(NOTHING_LOADED);
  const [loading, setLoading] = useState(Boolean(workspaceId));
  const [error, setError] = useState<string | null>(null);
  // Requests are ordered by issue, not by arrival: a slow first load must not
  // overwrite the result of a refresh the user triggered after it.
  const requestRef = useRef(0);

  const refresh = useCallback(() => {
    // Claim a request id unconditionally, including when there is no workspace.
    // Skipping it here let a response already in flight for the previous
    // workspace land afterwards and repopulate a feed that should be empty.
    const requestId = ++requestRef.current;

    if (!workspaceId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    listWorkspaceAutomationRuns(workspaceId, WORKSPACE_RUNS_LIMIT)
      .then((result) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ workspaceId, runs: result ?? EMPTY_RUNS });
        setError(null);
      })
      .catch((err: unknown) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ workspaceId, runs: EMPTY_RUNS });
        setError(err instanceof Error ? err.message : t("automations:failedToLoadRuns"));
      })
      .finally(() => {
        if (requestRef.current !== requestId) return;
        setLoading(false);
      });
  }, [workspaceId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Masked during render, not in an effect. A same-workspace refresh keeps the
  // rows the user is reading; a switch shows nothing until the new workspace's
  // own rows arrive.
  const switching = loaded.workspaceId !== workspaceId;
  const runs = switching ? EMPTY_RUNS : loaded.runs;
  const staleError = switching ? null : error;
  // Loading is derived for the same reason the rows are. The fetch is issued
  // from a passive effect, so on a switch there is a render where nothing has
  // loaded and nothing has been requested yet — reporting "not loading" there
  // flashes the empty state before the request even starts.
  const pending = loading || (switching && Boolean(workspaceId));

  return { runs, loading: pending, error: staleError, refresh };
}
