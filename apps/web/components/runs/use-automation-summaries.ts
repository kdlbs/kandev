"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listAutomationSummaries } from "@/lib/api/domains/automation-api";
import type { AutomationSummary } from "@/lib/types/automation";
import { t } from "@/lib/i18n";

const EMPTY_SUMMARIES: AutomationSummary[] = [];

/**
 * Summaries are stored with the workspace they were fetched for.
 *
 * Same reasoning as the runs feed: showing one workspace's health under
 * another's automations — even for the frame between a switch and the refetch
 * — reports the wrong thing about the wrong automation.
 */
type LoadedSummaries = {
  workspaceId: string;
  summaries: AutomationSummary[];
};

const NOTHING_LOADED: LoadedSummaries = { workspaceId: "", summaries: EMPTY_SUMMARIES };

export function useAutomationSummaries(workspaceId: string | undefined) {
  const [loaded, setLoaded] = useState<LoadedSummaries>(NOTHING_LOADED);
  const [loading, setLoading] = useState(Boolean(workspaceId));
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(() => {
    const requestId = ++requestRef.current;
    if (!workspaceId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    listAutomationSummaries(workspaceId)
      .then((summaries) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ workspaceId, summaries: summaries ?? EMPTY_SUMMARIES });
        setError(null);
      })
      .catch((err: unknown) => {
        if (requestRef.current !== requestId) return;
        setLoaded((current) =>
          current.workspaceId === workspaceId
            ? current
            : { workspaceId, summaries: EMPTY_SUMMARIES },
        );
        setError(
          err instanceof Error ? err.message : t("automations:failedToLoadAutomationActivity"),
        );
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
  return {
    summaries: switching ? EMPTY_SUMMARIES : loaded.summaries,
    loading: loading || (switching && Boolean(workspaceId)),
    error: switching ? null : error,
    refresh,
  };
}
