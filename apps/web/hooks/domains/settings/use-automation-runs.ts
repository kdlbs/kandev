"use client";

import { useEffect, useCallback } from "react";
import { toast } from "sonner";
import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { useAppStore } from "@/components/state-provider";
import type { AutomationRun } from "@/lib/types/automation";

const EMPTY_RUNS: AutomationRun[] = [];

export function useAutomationRuns(automationId: string | null, workspaceId: string) {
  const runs = useAppStore((state) =>
    automationId ? (state.automationRuns.byAutomationId[automationId] ?? EMPTY_RUNS) : EMPTY_RUNS,
  );
  const loading = useAppStore((state) =>
    automationId ? (state.automationRuns.loading[automationId] ?? false) : false,
  );
  const setRuns = useAppStore((state) => state.setAutomationRuns);
  const setRunsLoading = useAppStore((state) => state.setAutomationRunsLoading);
  const removeRun = useAppStore((state) => state.removeAutomationRun);
  const clearRuns = useAppStore((state) => state.clearAutomationRuns);

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

  const deleteRun = useCallback(
    (runId: string) => {
      if (!automationId) return;
      removeRun(automationId, runId); // optimistic
      deleteAutomationRun(runId, workspaceId)
        .then(() => {
          // Re-apply the removal: an in-flight refresh() / initial load can
          // resolve between the optimistic removeRun above and this success
          // callback and overwrite the store with the pre-delete list,
          // resurrecting the row. Removing it again here is a no-op unless
          // that happened.
          removeRun(automationId, runId);
        })
        .catch((err: unknown) => {
          const msg = err instanceof Error ? err.message : "Failed to delete run";
          toast.error(msg);
          // revert on failure
          listAutomationRuns(automationId)
            .then((result) => setRuns(automationId, result ?? []))
            .catch(() => {});
        });
    },
    [automationId, removeRun, setRuns, workspaceId],
  );

  const deleteAllRuns = useCallback(() => {
    if (!automationId) return;
    clearRuns(automationId); // optimistic
    deleteAllAutomationRuns(automationId, workspaceId)
      .then(() => {
        // See deleteRun: guard against an in-flight refresh() resurrecting
        // rows between the optimistic clear and this success callback.
        clearRuns(automationId);
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : "Failed to delete runs";
        toast.error(msg);
        // revert on failure
        listAutomationRuns(automationId)
          .then((result) => setRuns(automationId, result ?? []))
          .catch(() => {});
      });
  }, [automationId, clearRuns, setRuns, workspaceId]);

  return { runs, loading, refresh, deleteRun, deleteAllRuns };
}
