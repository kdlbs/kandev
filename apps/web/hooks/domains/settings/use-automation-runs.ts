"use client";

import { useCallback, useEffect } from "react";
import { t } from "@/lib/i18n";
import { toast } from "@/lib/toast/sonner";
import {
  listAutomationRuns,
  deleteAutomationRun,
  deleteAllAutomationRuns,
} from "@/lib/api/domains/automation-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AutomationRun } from "@/lib/types/automation";

const EMPTY_RUNS: AutomationRun[] = [];

const COULD_NOT_REFRESH_RUNS = "automations:couldNotRefreshRuns";

/**
 * Latest in-flight list request per (store instance, automation). `fetchRuns`
 * applies results and settles the shared `loading` flag only when the
 * finishing request is still the latest, so an older request completing after
 * a newer one started cannot overwrite the newer result or clear its loading
 * state. Tokens are opaque symbols — never reused, even after cleanup — and
 * the registry is scoped per store instance, so independent stores cannot
 * conflate each other's requests.
 */
const latestListRequest = new WeakMap<object, Map<string, symbol>>();

function registerListRequest(storeApi: object, automationId: string): symbol {
  let byAutomation = latestListRequest.get(storeApi);
  if (!byAutomation) {
    byAutomation = new Map();
    latestListRequest.set(storeApi, byAutomation);
  }
  const token = Symbol();
  byAutomation.set(automationId, token);
  return token;
}

function isLatestListRequest(storeApi: object, automationId: string, token: symbol): boolean {
  return latestListRequest.get(storeApi)?.get(automationId) === token;
}

function clearLatestListRequest(storeApi: object, automationId: string, token: symbol): void {
  const byAutomation = latestListRequest.get(storeApi);
  if (byAutomation?.get(automationId) === token) {
    byAutomation.delete(automationId);
  }
}

type FetchRunsOptions = {
  getEpoch: () => number;
  setRunsLoading: (automationId: string, loading: boolean) => void;
  setRuns: (automationId: string, runs: AutomationRun[]) => void;
  onError?: () => void;
  onSettled?: () => void;
};

function fetchRuns(storeApi: object, automationId: string, options: FetchRunsOptions): void {
  const { getEpoch, setRunsLoading, setRuns, onError, onSettled } = options;
  const captured = getEpoch();
  const token = registerListRequest(storeApi, automationId);
  setRunsLoading(automationId, true);
  listAutomationRuns(automationId)
    .then((result) => {
      // A newer request superseded this one: its result owns the store.
      if (!isLatestListRequest(storeApi, automationId, token)) return;
      // Discard responses that went stale while in flight: a delete that
      // started after this fetch captured the epoch owns the store now.
      if (getEpoch() === captured) setRuns(automationId, result ?? []);
    })
    .catch(() => {
      if (!isLatestListRequest(storeApi, automationId, token)) return;
      if (getEpoch() === captured) onError?.();
    })
    .finally(() => {
      if (isLatestListRequest(storeApi, automationId, token)) {
        clearLatestListRequest(storeApi, automationId, token);
        setRunsLoading(automationId, false);
      }
      onSettled?.();
    });
}

/**
 * Shared revert for the delete paths: on failure, refresh from the server
 * (authoritative — it drops whatever actually succeeded); if that also fails,
 * fall back to the pre-delete snapshot so the store is never left
 * permanently missing rows the delete never removed. Routed through
 * `fetchRuns`, so the recovery participates in loading state, request
 * ordering, and the epoch guard, and the serialization slot is held until
 * the recovery settles.
 */
function revertAfterFailedDelete(
  automationId: string,
  fallbackRuns: AutomationRun[],
  store: DeleteStore,
): void {
  // The delete has settled: mark every list response captured while it was
  // in flight stale — those may hold rows the delete removed server-side.
  store.bumpEpoch();
  fetchRuns(store.storeApi, automationId, {
    getEpoch: store.getEpoch,
    setRunsLoading: store.setRunsLoading,
    setRuns: store.setRuns,
    onError: () => {
      store.setRuns(automationId, fallbackRuns);
      toast.error(t(COULD_NOT_REFRESH_RUNS));
    },
    onSettled: store.endDelete,
  });
}

type DeleteStore = {
  storeApi: object;
  getEpoch: () => number;
  bumpEpoch: () => void;
  clearRuns: (automationId: string) => void;
  removeRun: (automationId: string, runId: string) => void;
  restoreRun: (automationId: string, run: AutomationRun) => void;
  setRuns: (automationId: string, runs: AutomationRun[]) => void;
  setRunsLoading: (automationId: string, loading: boolean) => void;
  getRuns: () => AutomationRun[];
  getRun: (runId: string) => AutomationRun | undefined;
  endDelete: () => void;
};

/**
 * Delete-all, scoped to the given run ids when provided. The caller has
 * already claimed the per-automation serialization slot (the epoch was
 * bumped and the generation recorded), so recoveries can never race a newer
 * mutation; the epoch still guards list fetches that predate the delete. On
 * success the store is reconciled with an authoritative post-delete refresh
 * instead of an unconditional clear, so a run created after the backend
 * delete completed survives.
 */
function executeDeleteAll(
  automationId: string,
  workspaceId: string,
  runIds: string[] | undefined,
  store: DeleteStore,
): void {
  if (runIds && runIds.length === 0) return;
  const previousRuns = store.getRuns();
  if (!runIds) {
    // Unscoped delete-all: remove every run for the automation in one call.
    store.clearRuns(automationId); // optimistic
    deleteAllAutomationRuns(automationId, workspaceId)
      .then(() => {
        // Settled: invalidate list responses captured while the delete was
        // in flight (they may hold rows the delete removed), then reconcile
        // with the authoritative post-delete list. The serialization slot
        // stays held until the reconciliation settles.
        store.bumpEpoch();
        fetchRuns(store.storeApi, automationId, {
          getEpoch: store.getEpoch,
          setRunsLoading: store.setRunsLoading,
          setRuns: store.setRuns,
          // Surface a failed reconciliation instead of silently keeping the
          // optimistic state; the delete itself already succeeded.
          onError: () => toast.error(t(COULD_NOT_REFRESH_RUNS)),
          onSettled: store.endDelete,
        });
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : t("automations:failedToDeleteRuns");
        toast.error(msg);
        revertAfterFailedDelete(automationId, previousRuns, store);
      });
    return;
  }
  // Status-scoped delete-all: remove exactly the given runs through the
  // per-run API. Archived/Cancelled are read-time-derived statuses that only
  // exist in the loaded payload, so the visible set is the only exact scope.
  const ids = [...new Set(runIds)];
  ids.forEach((id) => store.removeRun(automationId, id)); // optimistic
  Promise.allSettled(ids.map((id) => deleteAutomationRun(id, workspaceId))).then((results) => {
    if (results.every((result) => result.status === "fulfilled")) {
      // Settled: invalidate list responses captured while the deletes were
      // in flight, then reconcile with the authoritative post-delete list:
      // an in-flight refresh may have returned pre-delete rows, and a run
      // created after the deletes completed must not be wiped. The
      // serialization slot stays held until the reconciliation settles.
      store.bumpEpoch();
      fetchRuns(store.storeApi, automationId, {
        getEpoch: store.getEpoch,
        setRunsLoading: store.setRunsLoading,
        setRuns: store.setRuns,
        // Surface a failed reconciliation instead of silently keeping the
        // optimistic state; the deletes themselves already succeeded.
        onError: () => toast.error(t(COULD_NOT_REFRESH_RUNS)),
        onSettled: store.endDelete,
      });
      return;
    }
    // One toast and one recovery refresh for the whole batch, and only after
    // every delete has settled: recovering on the first rejection would race
    // the still-pending deletes and could resurrect rows that later succeed.
    const first = results.find((r): r is PromiseRejectedResult => r.status === "rejected");
    const msg =
      first && first.reason instanceof Error
        ? first.reason.message
        : t("automations:failedToDeleteRuns");
    toast.error(msg);
    // The double-failure fallback must not resurrect runs the server actually
    // deleted in this batch: drop the succeeded ids from the pre-delete
    // snapshot, keeping the failed rows and every untargeted row.
    const succeededIds = new Set(ids.filter((_, i) => results[i].status === "fulfilled"));
    revertAfterFailedDelete(
      automationId,
      previousRuns.filter((run) => !succeededIds.has(run.id)),
      store,
    );
  });
}

/**
 * Single-run delete. Serialized with delete-all like every other destructive
 * mutation (the caller claimed the slot); the epoch bump keeps pre-delete
 * list fetches from resurrecting the row, and the failure recovery restores
 * precisely the removed run when even the recovery refresh fails.
 */
function executeDeleteRun(
  automationId: string,
  workspaceId: string,
  runId: string,
  store: DeleteStore,
): void {
  // Snapshot the run being removed so we can restore it precisely (not the
  // whole list, which could clobber unrelated concurrent changes) if both
  // the delete and the recovery refresh below fail.
  const deletedRun = store.getRun(runId);
  store.removeRun(automationId, runId); // optimistic
  deleteAutomationRun(runId, workspaceId)
    .then(() => {
      // Re-apply the removal: an in-flight refresh() / initial load can
      // resolve between the optimistic removeRun above and this success
      // callback and overwrite the store with the pre-delete list,
      // resurrecting the row. Removing it again here is a no-op unless
      // that happened; the epoch bump then invalidates any list response
      // captured while the delete was in flight.
      store.removeRun(automationId, runId);
      store.bumpEpoch();
      store.endDelete();
    })
    .catch((err: unknown) => {
      // An Error message here is an API diagnostic and stays English; the
      // fallback for a non-Error rejection is copy.
      const msg = err instanceof Error ? err.message : t("automations:failedToDeleteRun");
      toast.error(msg);
      store.bumpEpoch();
      fetchRuns(store.storeApi, automationId, {
        getEpoch: store.getEpoch,
        setRunsLoading: store.setRunsLoading,
        setRuns: store.setRuns,
        onError: () => {
          // The recovery refresh also failed — the store would otherwise
          // stay permanently missing this row even though the delete never
          // succeeded server-side. Fall back to re-inserting just the run we
          // know we removed.
          if (deletedRun) store.restoreRun(automationId, deletedRun);
          toast.error(t(COULD_NOT_REFRESH_RUNS));
        },
        onSettled: store.endDelete,
      });
    });
}

export function useAutomationRuns(automationId: string | null, workspaceId: string) {
  const runs = useAppStore((state) =>
    automationId ? (state.automationRuns.byAutomationId[automationId] ?? EMPTY_RUNS) : EMPTY_RUNS,
  );
  const loading = useAppStore((state) =>
    automationId ? (state.automationRuns.loading[automationId] ?? false) : false,
  );
  const deleting = useAppStore((state) =>
    automationId ? (state.automationRuns.deleting[automationId] ?? false) !== false : false,
  );
  const setRuns = useAppStore((state) => state.setAutomationRuns);
  const setRunsLoading = useAppStore((state) => state.setAutomationRunsLoading);
  const removeRun = useAppStore((state) => state.removeAutomationRun);
  const clearRuns = useAppStore((state) => state.clearAutomationRuns);
  const restoreRun = useAppStore((state) => state.restoreAutomationRun);
  const storeApi = useAppStoreApi();

  // Destructive mutations are serialized per automation in shared store
  // state: only one may be in flight, so a recovery can never race a newer
  // mutation — even across hook instances (an editor can unmount mid-delete
  // and remount). `deleting` is exposed so the UI disables the delete
  // controls while one runs.
  const beginDelete = useCallback(() => {
    if (!automationId) return null;
    return storeApi.getState().beginAutomationRunDelete(automationId);
  }, [automationId, storeApi]);
  const endDelete = useCallback(
    (generation: number) => {
      if (!automationId) return;
      storeApi.getState().endAutomationRunDelete(automationId, generation);
    },
    [automationId, storeApi],
  );

  useEffect(() => {
    if (!automationId || loading) return;
    // A delete owns the store while it is in flight: its reconciliation or
    // recovery is the only list request, so it can never be superseded by a
    // mount fetch whose failure would skip the recovery's snapshot restore.
    if ((storeApi.getState().automationRuns.deleting[automationId] ?? false) !== false) return;
    fetchRuns(storeApi, automationId, {
      getEpoch: () => storeApi.getState().automationRuns.mutationEpoch[automationId] ?? 0,
      setRunsLoading,
      setRuns,
      onError: () => setRuns(automationId, []),
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [automationId]);

  const refresh = useCallback(() => {
    if (!automationId) return;
    // See the mount effect: while a delete is in flight its reconciliation /
    // recovery owns the list state and must not be superseded.
    if ((storeApi.getState().automationRuns.deleting[automationId] ?? false) !== false) return;
    fetchRuns(storeApi, automationId, {
      getEpoch: () => storeApi.getState().automationRuns.mutationEpoch[automationId] ?? 0,
      setRunsLoading,
      setRuns,
    });
  }, [automationId, setRuns, setRunsLoading, storeApi]);

  const makeStore = useCallback(
    (end: () => void): DeleteStore => ({
      storeApi,
      getEpoch: () => storeApi.getState().automationRuns.mutationEpoch[automationId ?? ""] ?? 0,
      bumpEpoch: () => storeApi.getState().advanceAutomationRunEpoch(automationId ?? ""),
      clearRuns,
      removeRun,
      restoreRun,
      setRuns,
      setRunsLoading,
      getRuns: () => storeApi.getState().automationRuns.byAutomationId[automationId ?? ""] ?? [],
      getRun: (runId) =>
        storeApi
          .getState()
          .automationRuns.byAutomationId[automationId ?? ""]?.find((r) => r.id === runId),
      endDelete: end,
    }),
    [clearRuns, removeRun, restoreRun, setRuns, setRunsLoading, storeApi, automationId],
  );

  const deleteRun = useCallback(
    (runId: string) => {
      if (!automationId) return;
      const generation = beginDelete();
      if (generation === null) return;
      executeDeleteRun(
        automationId,
        workspaceId,
        runId,
        makeStore(() => endDelete(generation)),
      );
    },
    [automationId, beginDelete, endDelete, makeStore, workspaceId],
  );

  const deleteAllRuns = useCallback(
    (runIds?: string[]) => {
      if (!automationId) return;
      // An empty scope is a no-op: the UI never offers delete-all for an
      // empty view, and falling back to the all-runs delete here would be a
      // dangerous surprise for a caller that computed zero ids.
      if (runIds && runIds.length === 0) return;
      const generation = beginDelete();
      if (generation === null) return;
      executeDeleteAll(
        automationId,
        workspaceId,
        runIds,
        makeStore(() => endDelete(generation)),
      );
    },
    [automationId, beginDelete, endDelete, makeStore, workspaceId],
  );

  return { runs, loading, refresh, deleteRun, deleteAllRuns, deleting };
}
