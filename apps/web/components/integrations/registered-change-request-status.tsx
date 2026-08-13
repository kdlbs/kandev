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
import type { PluginReviewProviderRegistration } from "@/lib/plugins/registry";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { IntegrationChangeRequestStatus } from "./integration-change-request-status";
import type { IntegrationChangeRequestStatusItem } from "./integration-change-request-status-types";

const STATUS_REFRESH_INTERVAL_MS = 90_000;

type RegisteredChangeRequestStatusProps = {
  taskId: string | null;
  sessionId?: string | null;
  surface: "topbar" | "composer";
};

function usePeriodicStatusRefresh(taskId: string | null, reviews: readonly ReviewItemSummary[]) {
  const registry = usePluginRegistry();
  useEffect(() => {
    if (!taskId || reviews.length === 0) return;
    const providerIds = Array.from(new Set(reviews.map((review) => review.providerId)));
    const interval = setInterval(() => {
      providerIds.forEach((providerId) => {
        const provider = registry.getReviewProvider(providerId);
        if (provider) void refreshReviewProvider(provider, taskId);
      });
    }, STATUS_REFRESH_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [registry, reviews, taskId]);
}

function createUnlinkHandler({
  provider,
  workspaceId,
  taskId,
  review,
  toast,
}: {
  provider: PluginReviewProviderRegistration;
  workspaceId: string;
  taskId: string;
  review: ReviewItemSummary;
  toast: ReturnType<typeof useToast>["toast"];
}) {
  if (!provider.unlink) return undefined;
  return async (signal: AbortSignal) => {
    try {
      await provider.unlink!({
        workspaceId,
        taskId,
        reviewKey: review.reviewKey,
        connectionScope: review.connectionScope,
        repositoryId: review.repositoryId,
        changeRequestNumber: review.changeRequestNumber,
        signal,
      });
      await Promise.all([
        refreshReviewProvider(provider, taskId),
        provider.refreshAssociations?.(workspaceId, new AbortController().signal),
      ]);
      toast({ title: t("integrations:pullRequestUnlinked"), variant: "success" });
    } catch (error) {
      toast({
        title: t("integrations:failedToUnlinkPullRequest"),
        description: error instanceof Error ? error.message : t("integrations:unknownError"),
        variant: "error",
      });
    }
  };
}

function useRegisteredStatusItems({
  taskId,
  sessionId,
  reviews,
  loading,
}: Pick<RegisteredChangeRequestStatusProps, "taskId" | "sessionId"> & {
  reviews: readonly ReviewItemSummary[];
  loading: boolean;
}) {
  const registry = usePluginRegistry();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { toast } = useToast();
  const usesTouchDrawer = useTouchDrawer();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setMobileSessionReview = useAppStore((state) => state.setMobileSessionReview);
  const addReviewPanel = useDockviewStore((state) => state.addReviewPanel);
  return useMemo<IntegrationChangeRequestStatusItem[]>(
    () =>
      reviews.flatMap((review) => {
        const status = review.taskStatus;
        if (!status) return [];
        const provider = registry.getReviewProvider(review.providerId);
        const onUnlink =
          provider && workspaceId && taskId
            ? createUnlinkHandler({
                provider,
                workspaceId,
                taskId,
                review,
                toast,
              })
            : undefined;
        return [
          {
            id: reviewItemId(review),
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
            ...(onUnlink ? { onUnlink } : {}),
            onOpenReview: () => {
              const mobileSessionId = sessionId ?? activeSessionId;
              if (usesTouchDrawer && mobileSessionId) {
                setMobileSessionReview(mobileSessionId, reviewItemId(review));
                return;
              }
              addReviewPanel(review);
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
      reviews,
      taskId,
      toast,
      usesTouchDrawer,
      workspaceId,
    ],
  );
}

export function RegisteredChangeRequestStatus({
  taskId,
  sessionId,
  surface,
}: RegisteredChangeRequestStatusProps) {
  const { reviews, loading } = useNormalizedTaskReviewsState(taskId);
  const statusReviews = useMemo(() => reviews.filter((review) => review.taskStatus), [reviews]);
  usePeriodicStatusRefresh(taskId, statusReviews);
  const items = useRegisteredStatusItems({ taskId, sessionId, reviews: statusReviews, loading });
  return <IntegrationChangeRequestStatus items={items} surface={surface} />;
}
