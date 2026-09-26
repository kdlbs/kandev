"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getAutomation,
  getAutomationSummary,
  listAutomationRuns,
} from "@/lib/api/domains/automation-api";
import type { Automation, AutomationRun, AutomationSummary } from "@/lib/types/automation";
import { t } from "@/lib/i18n";

/**
 * The detail view reads one automation's history. It is a reading surface, not
 * an audit trail, so the same reasoning as the workspace feed applies — a
 * bounded window, no pagination.
 */
export const AUTOMATION_RUNS_LIMIT = 100;

const EMPTY_RUNS: AutomationRun[] = [];

/**
 * The automation and its runs are stored with the id they were fetched for.
 *
 * Both tabs of the detail view render off this, so showing another
 * automation's runs under this one's name — even for the frame between a
 * navigation and the effect that refetches — would be a straightforward lie
 * about whose history the user is reading.
 */
type LoadedActivity = {
  automationId: string;
  automation: Automation | null;
  runs: AutomationRun[];
  /**
   * The server's count of this automation's outstanding runs. Not derived from
   * `runs`: that list is capped, so an open run older than the window would
   * read as zero — the page would report nothing in flight and stop polling.
   */
  openRuns: number;
};

const NOTHING_LOADED: LoadedActivity = {
  automationId: "",
  automation: null,
  runs: EMPTY_RUNS,
  openRuns: 0,
};

export function useAutomationActivity(automationId: string) {
  const [loaded, setLoaded] = useState<LoadedActivity>(NOTHING_LOADED);
  const [loading, setLoading] = useState(Boolean(automationId));
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(() => {
    const requestId = ++requestRef.current;
    if (!automationId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    // One await for both: the header needs the schedule and the activity list
    // needs the runs, and a page that renders half of itself first reflows
    // under the reader.
    Promise.all([
      getAutomation(automationId),
      listAutomationRuns(automationId, AUTOMATION_RUNS_LIMIT),
      getAutomationSummary(automationId),
    ])
      .then(
        ([automation, runs, summary]: [Automation, AutomationRun[], AutomationSummary | null]) => {
          if (requestRef.current !== requestId) return;
          setLoaded({
            automationId,
            automation,
            runs: runs ?? EMPTY_RUNS,
            openRuns: summary?.open_runs ?? 0,
          });
          setError(null);
        },
      )
      .catch((err: unknown) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ automationId, automation: null, runs: EMPTY_RUNS, openRuns: 0 });
        setError(err instanceof Error ? err.message : t("automations:failedToLoadThisAutomation"));
      })
      .finally(() => {
        if (requestRef.current !== requestId) return;
        setLoading(false);
      });
  }, [automationId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const switching = loaded.automationId !== automationId;
  return {
    automation: switching ? null : loaded.automation,
    runs: switching ? EMPTY_RUNS : loaded.runs,
    openRuns: switching ? 0 : loaded.openRuns,
    loading: loading || (switching && Boolean(automationId)),
    error: switching ? null : error,
    refresh,
  };
}
