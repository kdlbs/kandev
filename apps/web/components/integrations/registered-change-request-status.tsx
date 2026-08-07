"use client";

import { useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  refreshReviewProvider,
  useNormalizedTaskReviewsState,
} from "@/components/task/review-panel-provider";
import { reviewItemId } from "@/components/task/review-selection";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { IntegrationChangeRequestStatus } from "./integration-change-request-status";
import type { IntegrationChangeRequestStatusItem } from "./integration-change-request-status-types";

const STATUS_REFRESH_INTERVAL_MS = 90_000;

export function RegisteredChangeRequestStatus({
  taskId,
  sessionId,
  surface,
}: {
  taskId: string | null;
  sessionId?: string | null;
  surface: "topbar" | "composer";
}) {
  const registry = usePluginRegistry();
  const { reviews, loading } = useNormalizedTaskReviewsState(taskId);
  const usesTouchDrawer = useTouchDrawer();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setMobileSessionReview = useAppStore((state) => state.setMobileSessionReview);
  const addReviewPanel = useDockviewStore((state) => state.addReviewPanel);
  const statusReviews = useMemo(() => reviews.filter((review) => review.taskStatus), [reviews]);

  useEffect(() => {
    if (!taskId || statusReviews.length === 0) return;
    const providerIds = Array.from(new Set(statusReviews.map((review) => review.providerId)));
    const interval = setInterval(() => {
      providerIds.forEach((providerId) => {
        const provider = registry.getReviewProvider(providerId);
        if (provider) void refreshReviewProvider(provider, taskId);
      });
    }, STATUS_REFRESH_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [registry, statusReviews, taskId]);

  const items = useMemo<IntegrationChangeRequestStatusItem[]>(
    () =>
      statusReviews.flatMap((review) => {
        const status = review.taskStatus;
        if (!status) return [];
        return [
          {
            id: `${review.providerId}:${review.reviewKey}`,
            number: status.number,
            title: review.title,
            repositoryLabel: review.repositoryId,
            url: review.url,
            state: status.state,
            status: status.pipelineState,
            pipelineRows: status.checks,
            review: status.review,
            unresolvedComments: status.unresolvedComments,
            loading: loading || status.loading,
            error: status.error,
            updatedAt: status.updatedAt,
            onRefresh: async () => {
              if (!taskId) return;
              const provider = registry.getReviewProvider(review.providerId);
              if (provider) await refreshReviewProvider(provider, taskId);
            },
            onOpenReview: () => {
              const mobileSessionId = sessionId ?? activeSessionId;
              if (usesTouchDrawer && mobileSessionId) {
                setMobileSessionReview(
                  mobileSessionId,
                  reviewItemId({
                    providerId: review.providerId,
                    reviewKey: review.reviewKey,
                  }),
                );
                return;
              }
              addReviewPanel(review.providerId, review.reviewKey, review.title);
            },
          },
        ];
      }),
    [
      activeSessionId,
      addReviewPanel,
      loading,
      registry,
      sessionId,
      setMobileSessionReview,
      statusReviews,
      taskId,
      usesTouchDrawer,
    ],
  );

  return <IntegrationChangeRequestStatus items={items} surface={surface} />;
}
