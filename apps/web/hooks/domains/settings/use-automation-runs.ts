"use client";

import { useEffect, useCallback } from "react";
import { listAutomationRuns } from "@/lib/api/domains/automation-api";
import { useAppStore } from "@/components/state-provider";
import type { AutomationRun } from "@/lib/types/automation";

const EMPTY_RUNS: AutomationRun[] = [];

export function useAutomationRuns(automationId: string | null) {
  const runs = useAppStore((state) =>
    automationId ? (state.automationRuns.byAutomationId[automationId] ?? EMPTY_RUNS) : EMPTY_RUNS,
  );
  const loading = useAppStore((state) =>
    automationId ? (state.automationRuns.loading[automationId] ?? false) : false,
  );
  const setRuns = useAppStore((state) => state.setAutomationRuns);
  const setRunsLoading = useAppStore((state) => state.setAutomationRunsLoading);

  useEffect(() => {
    if (!automationId || loading) return;
    setRunsLoading(automationId, true);
    listAutomationRuns(automationId)
      .then((result) => {
        setRuns(automationId, result ?? []);
      })
      .catch(() => {
        setRuns(automationId, []);
      })
      .finally(() => {
        setRunsLoading(automationId, false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [automationId]);

  const refresh = useCallback(() => {
    if (!automationId) return;
    setRunsLoading(automationId, true);
    listAutomationRuns(automationId)
      .then((result) => {
        setRuns(automationId, result ?? []);
      })
      .catch(() => {})
      .finally(() => {
        setRunsLoading(automationId, false);
      });
  }, [automationId, setRuns, setRunsLoading]);

  return { runs, loading, refresh };
}
