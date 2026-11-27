"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { updateReviewFindingStatus } from "@/lib/api/domains/review-api";
import { beginFindingAction } from "@/lib/review/finding-action-publication";
import type { ReviewFindingStatus, TaskReviewFinding } from "@/lib/types/review";

/**
 * Resolve / dismiss / reopen actions for a review finding.
 *
 * Overlapping local actions share publication ownership and an acknowledged
 * rollback baseline across all consumers of the same store and finding.
 */
export function useFindingActions(taskId: string | null | undefined) {
  const storeApi = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation("review");

  const setStatus = useCallback(
    async (finding: TaskReviewFinding, status: ReviewFindingStatus) => {
      if (!taskId) return;
      const settle = beginFindingAction(storeApi, taskId, finding.id, status);
      if (!settle) return;
      try {
        const updated = await updateReviewFindingStatus(finding.id, status);
        settle(updated);
      } catch (error) {
        settle();
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
