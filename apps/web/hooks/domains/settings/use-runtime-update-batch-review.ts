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
  onLoaded: (keys: Set<string>) => void;
};

function useRuntimeReviewLoader({
  active,
  batchStatus,
  candidates,
  preview,
  beginReview,
  onLoaded,
}: ReviewLoaderOptions) {
  const [previewFailures, setPreviewFailures] = useState<Record<string, string>>({});
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
    if (signature.current === candidateSignature && batchStatus !== "idle") return;
    signature.current = candidateSignature;
    const id = ++requestID.current;
    setReviewLoading(true);
    void loadRuntimeUpdateReviewRows(candidates, preview)
      .then(({ rows, failures }) => {
        if (id !== requestID.current) return;
        setPreviewFailures(failures);
        beginReview(rows);
        onLoaded(new Set(rows.map((row) => row.key)));
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
  const reviewRows =
    batch.status === "review"
      ? batch.order.map((key) => batch.rows[key]).filter((row) => row?.phase === "review")
      : [];
  const reviewKeys = new Set(reviewRows.filter(Boolean).map((row) => row.key));

  useEffect(() => {
    if (batch.status !== "review") return;
    setSelection((previous) => {
      const next = new Set([...previous].filter((key) => reviewKeys.has(key)));
      return next.size > 0 || touched.current ? next : new Set(reviewKeys);
    });
  }, [batch.status, batch.order]);

  const toggleSelected = useCallback((key: string, selected: boolean) => {
    touched.current = true;
    setSelection((previous) => {
      const next = new Set(previous);
      if (selected) next.add(key);
      else next.delete(key);
      return next;
    });
  }, []);

  const selectAll = useCallback(
    (selected: boolean) => {
      touched.current = true;
      setSelection(selected ? new Set(reviewKeys) : new Set());
    },
    [reviewKeys],
  );

  return {
    selection,
    setSelection,
    touched,
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
  setSelection: (value: ReadonlySet<string>) => void;
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
  setSelection,
  reviewRows,
  batch,
}: FreshRetryOptions) {
  const [retrying, setRetrying] = useState(false);

  const loadFreshRetryRows = useCallback(
    async (rows: RuntimeBatchRow[]) => {
      const statusByAgent = {
        ...Object.fromEntries(candidates.map((status) => [status.agent_name, status])),
        ...store.getState().agentRuntimeUpdates.byAgent,
      };
      const eligible: AgentUpdateStatus[] = [];
      const failures: Record<string, string> = {};
      for (const row of rows) {
        const status = statusByAgent[row.agentName];
        if (!status || !isBulkCandidateStatus(status) || (isEligible && !isEligible(status))) {
          failures[row.key] = "review_changed";
          continue;
        }
        eligible.push(status);
      }
      const result = await loadRuntimeUpdateReviewRows(eligible, preview);
      beginRetryReview(result.rows);
      loader.setPreviewFailures({ ...failures, ...result.failures });
      touched.current = false;
      setSelection(new Set(result.rows.map((row) => row.key)));
    },
    [beginRetryReview, candidates, isEligible, loader, preview, setSelection, store, touched],
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
    const hasSucceeded = batch.order.some((key) => batch.rows[key]?.phase === "succeeded");
    if (hasSucceeded) {
      void loadFreshRetryRows(reviewRows);
      return;
    }
    loader.retryPreviews();
  }, [batch.order, batch.rows, loader, loadFreshRetryRows, reviewRows]);

  return { retrying, retry, retryPreviews };
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
  const { selection, setSelection, touched, reviewRows, reviewKeys } = selectionState;
  const onLoaded = useCallback(
    (keys: Set<string>) => {
      setSelection((previous) => {
        if (!touched.current) return keys;
        return new Set([...previous].filter((key) => keys.has(key)));
      });
    },
    [setSelection, touched],
  );
  const loader = useRuntimeReviewLoader({
    active,
    batchStatus: batch.status,
    candidates,
    preview,
    beginReview,
    onLoaded,
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
    setSelection,
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
    loader.resetForNewReview();
  }, [loader, touched]);

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
