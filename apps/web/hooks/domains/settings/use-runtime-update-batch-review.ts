"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useAppStoreApi } from "@/components/state-provider";
import type { AgentUpdateStatus } from "@/lib/api";
import { isBulkCandidateStatus } from "@/lib/agents/runtime-update-manager";
import {
  loadRuntimeUpdateReviewRows,
  runtimeBatchKey,
  type RuntimeBatchRow,
  type RuntimeBatchTarget,
  type RuntimePreviewFailure,
  type RuntimePreviewLoader,
  type RuntimeUpdateBatchState,
} from "@/lib/agents/runtime-update-batch";
import { useAgentRuntimeUpdateBatch } from "./use-agent-runtime-update-batch";

type ReviewLoaderOptions = {
  active: boolean;
  batchStatus: RuntimeUpdateBatchState["status"];
  candidates: AgentUpdateStatus[];
  preview: RuntimePreviewLoader;
  beginReview: (rows: RuntimeBatchTarget[]) => boolean;
  onLoaded: (rows: RuntimeBatchTarget[]) => void;
};

function useRuntimeReviewLoader({
  active,
  batchStatus,
  candidates,
  preview,
  beginReview,
  onLoaded,
}: ReviewLoaderOptions) {
  const [previewFailures, setPreviewFailures] = useState<Record<string, RuntimePreviewFailure>>({});
  const [reviewLoading, setReviewLoading] = useState(false);
  const [retryEpoch, setRetryEpoch] = useState(0);
  const signature = useRef<string | null>(null);
  const requestID = useRef(0);
  const observedRun = useRef(false);
  const candidateSignature = candidates
    .map((status) => `${runtimeBatchKey(status)}@${status.latest_version ?? ""}`)
    .join("|");

  useEffect(() => {
    if (!active) return;
    if (batchStatus === "running") {
      observedRun.current = true;
      return;
    }
    if (observedRun.current && (batchStatus === "complete" || batchStatus === "review")) return;
    if (signature.current === candidateSignature) return;
    signature.current = candidateSignature;
    const id = ++requestID.current;
    setReviewLoading(true);
    void loadRuntimeUpdateReviewRows(candidates, preview)
      .then(({ rows, failures }) => {
        if (id !== requestID.current) return;
        setPreviewFailures(failures);
        beginReview(rows);
        onLoaded(rows);
      })
      .finally(() => {
        if (id === requestID.current) setReviewLoading(false);
      });
  }, [
    active,
    batchStatus,
    beginReview,
    candidateSignature,
    candidates,
    preview,
    retryEpoch,
    onLoaded,
  ]);

  const retryPreviews = useCallback(() => {
    signature.current = null;
    setRetryEpoch((epoch) => epoch + 1);
  }, []);

  const resetForNewReview = useCallback(() => {
    observedRun.current = false;
    signature.current = null;
    setRetryEpoch((epoch) => epoch + 1);
  }, []);

  return { previewFailures, setPreviewFailures, reviewLoading, retryPreviews, resetForNewReview };
}

function useReviewSelection(batch: RuntimeUpdateBatchState) {
  const [selection, setSelection] = useState<ReadonlySet<string>>(new Set());
  const touched = useRef(false);
  const selectedTargets = useRef(new Map<string, string>());
  const reviewRows =
    batch.status === "review"
      ? batch.order.map((key) => batch.rows[key]).filter((row) => row?.phase === "review")
      : [];
  const reviewKeys = new Set(reviewRows.filter(Boolean).map((row) => row.key));

  useEffect(() => {
    if (batch.status !== "review") return;
    setSelection((previous) => {
      const next = new Set([...previous].filter((key) => reviewKeys.has(key)));
      for (const key of selectedTargets.current.keys()) {
        if (!next.has(key)) selectedTargets.current.delete(key);
      }
      return next.size > 0 || touched.current ? next : new Set(reviewKeys);
    });
  }, [batch.status, batch.order]);

  const rememberLoaded = useCallback((rows: RuntimeBatchTarget[]) => {
    const rowTargets = new Map(rows.map((row) => [row.key, row.targetVersion]));
    for (const key of selectedTargets.current.keys()) {
      if (!rowTargets.has(key)) selectedTargets.current.delete(key);
    }
    setSelection((previous) => {
      if (!touched.current) {
        selectedTargets.current = rowTargets;
        return new Set(rowTargets.keys());
      }
      const next = new Set(
        [...previous].filter(
          (key) => rowTargets.has(key) && selectedTargets.current.get(key) === rowTargets.get(key),
        ),
      );
      for (const key of selectedTargets.current.keys()) {
        if (!next.has(key)) selectedTargets.current.delete(key);
      }
      return next;
    });
  }, []);

  const replaceSelection = useCallback((rows: RuntimeBatchTarget[]) => {
    selectedTargets.current = new Map(rows.map((row) => [row.key, row.targetVersion]));
    setSelection(new Set(rows.map((row) => row.key)));
  }, []);

  const toggleSelected = useCallback(
    (key: string, selected: boolean) => {
      touched.current = true;
      const targetVersion = reviewRows.find((row) => row.key === key)?.targetVersion;
      setSelection((previous) => {
        const next = new Set(previous);
        if (selected) {
          next.add(key);
          if (targetVersion) selectedTargets.current.set(key, targetVersion);
        } else {
          next.delete(key);
          selectedTargets.current.delete(key);
        }
        return next;
      });
    },
    [reviewRows],
  );

  const selectAll = useCallback(
    (selected: boolean) => {
      touched.current = true;
      if (selected) {
        selectedTargets.current = new Map(reviewRows.map((row) => [row.key, row.targetVersion]));
        setSelection(new Set(reviewKeys));
      } else {
        selectedTargets.current.clear();
        setSelection(new Set());
      }
    },
    [reviewKeys, reviewRows],
  );

  return {
    selection,
    touched,
    rememberLoaded,
    replaceSelection,
    reviewRows: reviewRows.filter((row): row is RuntimeBatchRow => Boolean(row)),
    reviewKeys,
    toggleSelected,
    selectAll,
  };
}

type FreshRetryOptions = {
  store: ReturnType<typeof useAppStoreApi>;
  candidates: AgentUpdateStatus[];
  preview: RuntimePreviewLoader;
  isEligible?: (status: AgentUpdateStatus) => boolean;
  beginRetryReview: (targets: RuntimeBatchTarget[]) => boolean;
  refreshStatus: () => Promise<void>;
  loader: ReturnType<typeof useRuntimeReviewLoader>;
  touched: { current: boolean };
  replaceSelection: (rows: RuntimeBatchTarget[]) => void;
  reviewRows: RuntimeBatchRow[];
  batch: RuntimeUpdateBatchState;
};

function useFreshRetryReview({
  store,
  candidates,
  preview,
  isEligible,
  beginRetryReview,
  refreshStatus,
  loader,
  touched,
  replaceSelection,
  reviewRows,
  batch,
}: FreshRetryOptions) {
  const [retrying, setRetrying] = useState(false);
  const pendingPreviewRows = useRef<RuntimeBatchRow[]>([]);
  const retryActive = useRef(false);

  const loadFreshRetryRows = useCallback(
    async (rows: RuntimeBatchRow[]) => {
      const statusByAgent = {
        ...Object.fromEntries(candidates.map((status) => [status.agent_name, status])),
        ...store.getState().agentRuntimeUpdates.byAgent,
      };
      const eligible: AgentUpdateStatus[] = [];
      const failures: Record<string, RuntimePreviewFailure> = {};
      for (const row of rows) {
        const status = statusByAgent[row.agentName];
        if (!status || !isBulkCandidateStatus(status) || (isEligible && !isEligible(status))) {
          failures[row.key] = { code: "runtime_changed" };
          continue;
        }
        eligible.push(status);
      }
      const result = await loadRuntimeUpdateReviewRows(eligible, preview);
      beginRetryReview(result.rows);
      const freshKeys = new Set(result.rows.map((row) => row.key));
      pendingPreviewRows.current = rows.filter((row) => !freshKeys.has(row.key));
      loader.setPreviewFailures({ ...failures, ...result.failures });
      touched.current = false;
      replaceSelection(result.rows);
      retryActive.current = true;
    },
    [beginRetryReview, candidates, isEligible, loader, preview, replaceSelection, store, touched],
  );

  const retry = useCallback(async () => {
    const failedRows = batch.order
      .map((key) => batch.rows[key])
      .filter((row) => row?.phase === "failed");
    if (failedRows.length === 0) return false;
    setRetrying(true);
    try {
      await refreshStatus();
      await loadFreshRetryRows(failedRows);
      return true;
    } finally {
      setRetrying(false);
    }
  }, [batch.order, batch.rows, loadFreshRetryRows, refreshStatus]);

  const retryPreviews = useCallback(() => {
    if (retryActive.current) {
      const byKey = new Map<string, RuntimeBatchRow>();
      for (const row of [...pendingPreviewRows.current, ...reviewRows]) byKey.set(row.key, row);
      void loadFreshRetryRows([...byKey.values()]);
      return;
    }
    loader.retryPreviews();
  }, [loader, loadFreshRetryRows, reviewRows]);

  const resetRetry = useCallback(() => {
    retryActive.current = false;
    pendingPreviewRows.current = [];
  }, []);

  return { retrying, retry, retryPreviews, resetRetry };
}

/**
 * Owns the review lifecycle around the store coordinator: exact previews at
 * most two at a time, row-level failures, and selection that never silently
 * absorbs newly discovered candidates.
 */
export function useRuntimeUpdateReview({
  active,
  candidates,
  preview,
  isEligible,
}: {
  active: boolean;
  candidates: AgentUpdateStatus[];
  preview: RuntimePreviewLoader;
  isEligible?: (status: AgentUpdateStatus) => boolean;
}) {
  const store = useAppStoreApi();
  const { batch, beginReview, beginRetryReview, start, refreshStatus } =
    useAgentRuntimeUpdateBatch();
  const selectionState = useReviewSelection(batch);
  const { selection, touched, reviewRows, reviewKeys } = selectionState;
  const loader = useRuntimeReviewLoader({
    active,
    batchStatus: batch.status,
    candidates,
    preview,
    beginReview,
    onLoaded: selectionState.rememberLoaded,
  });
  const freshRetry = useFreshRetryReview({
    store,
    candidates,
    preview,
    isEligible,
    beginRetryReview,
    refreshStatus,
    loader,
    touched,
    replaceSelection: selectionState.replaceSelection,
    reviewRows,
    batch,
  });

  const startSelected = useCallback(
    (eligibleKeys: ReadonlySet<string>) => {
      touched.current = false;
      return start(selection, eligibleKeys);
    },
    [selection, start, touched],
  );
  const resetAfterCompletion = useCallback(() => {
    touched.current = false;
    freshRetry.resetRetry();
    loader.resetForNewReview();
  }, [freshRetry, loader, touched]);

  return {
    batch,
    previewFailures: loader.previewFailures,
    reviewLoading: loader.reviewLoading || freshRetry.retrying,
    reviewRows,
    reviewKeys,
    selection,
    toggleSelected: selectionState.toggleSelected,
    selectAll: selectionState.selectAll,
    startSelected,
    retryFailed: freshRetry.retry,
    retryPreviews: freshRetry.retryPreviews,
    resetAfterCompletion,
    refreshStatus,
    allSelected: reviewKeys.size > 0 && selection.size === reviewKeys.size,
  };
}
