"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { updateReviewFindingStatus } from "@/lib/api/domains/review-api";
import type { ReviewFindingStatus, TaskReviewFinding } from "@/lib/types/review";

/**
 * Resolve / dismiss / reopen actions for a review finding.
 *
 * Updates optimistically so the card responds immediately, and rolls back on
 * failure — a finding that silently stays open after the user resolved it would
 * misrepresent the review's state.
 */
export function useFindingActions(taskId: string | null | undefined) {
  const storeApi = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation("review");

  const setStatus = useCallback(
    async (finding: TaskReviewFinding, status: ReviewFindingStatus) => {
      if (!taskId) return;
      const previous = finding;
      storeApi.getState().updateReviewFinding(taskId, { ...finding, status });
      try {
        const updated = await updateReviewFindingStatus(finding.id, status);
        storeApi.getState().updateReviewFinding(taskId, updated);
      } catch (error) {
        storeApi.getState().updateReviewFinding(taskId, previous);
        toast({
          title: t("review:couldNotUpdateFinding"),
          description: error instanceof Error ? error.message : t("common:anErrorOccurred"),
          variant: "error",
        });
      }
    },
    [taskId, storeApi, t, toast],
  );

  return {
    resolveFinding: useCallback(
      (finding: TaskReviewFinding) => setStatus(finding, "resolved"),
      [setStatus],
    ),
    dismissFinding: useCallback(
      (finding: TaskReviewFinding) => setStatus(finding, "dismissed"),
      [setStatus],
    ),
    reopenFinding: useCallback(
      (finding: TaskReviewFinding) => setStatus(finding, "open"),
      [setStatus],
    ),
  };
}
