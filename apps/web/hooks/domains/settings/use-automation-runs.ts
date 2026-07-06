"use client";

import { useEffect, useCallback } from "react";
import { toast } from "sonner";
import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
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
  const restoreRun = useAppStore((state) => state.restoreAutomationRun);
  const storeApi = useAppStoreApi();

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
      // Snapshot the run being removed so we can restore it precisely (not
      // the whole list, which could clobber unrelated concurrent changes)
      // if both the delete and the recovery refresh below fail.
      const deletedRun = storeApi
        .getState()
        .automationRuns.byAutomationId[automationId]?.find((r) => r.id === runId);
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
            .catch(() => {
              // The recovery refresh also failed — the store would
              // otherwise stay permanently missing this row even though
              // the delete never succeeded server-side. Fall back to
              // re-inserting just the run we know we removed.
              if (deletedRun) {
                restoreRun(automationId, deletedRun);
              }
              toast.error("Could not refresh runs — restored from local cache");
            });
        });
    },
    [automationId, removeRun, restoreRun, setRuns, storeApi, workspaceId],
  );

  const deleteAllRuns = useCallback(() => {
    if (!automationId) return;
    // Snapshot the full list so we can restore it if both the delete-all
    // and the recovery refresh below fail.
    const previousRuns = storeApi.getState().automationRuns.byAutomationId[automationId] ?? [];
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
          .catch(() => {
            // The recovery refresh also failed — without this the store
            // would stay permanently empty even though delete-all never
            // succeeded server-side. Fall back to the pre-clear snapshot.
            setRuns(automationId, previousRuns);
            toast.error("Could not refresh runs — restored from local cache");
          });
      });
  }, [automationId, clearRuns, setRuns, storeApi, workspaceId]);

  return { runs, loading, refresh, deleteRun, deleteAllRuns };
}
