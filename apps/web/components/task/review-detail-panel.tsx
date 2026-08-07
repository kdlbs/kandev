"use client";

import { useMemo } from "react";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { MRDetailPanelComponent } from "@/components/gitlab/mr-detail-panel";
import { useAppStore } from "@/components/state-provider";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginReviewProviderRegistration } from "@/lib/plugins/registry";
import {
  resolveReviewKey,
  resolveReviewPanelProvider,
  useNormalizedTaskReviews,
  useReviewProviderUpdates,
} from "./review-panel-provider";
import { ReviewItemSelector } from "./review-item-selector";
import { useReviewItemSelection } from "./review-selection";

function RegisteredReviewPanel({
  panelId,
  provider,
  taskId,
  workspaceId,
  sessionId,
  reviewKey,
  presentation,
}: {
  panelId: string;
  provider: PluginReviewProviderRegistration;
  taskId: string;
  workspaceId: string;
  sessionId: string | null;
  reviewKey: string;
  presentation: "desktop" | "mobile";
}) {
  const providers = useMemo(() => [provider], [provider]);
  const version = useReviewProviderUpdates(taskId, providers);
  const items = provider.getSnapshot(taskId);
  const selected = items.find((item) => item.reviewKey === reviewKey);

  if (!selected) {
    const EmptyState = provider.EmptyState;
    return EmptyState ? <EmptyState /> : <ReviewUnavailable />;
  }
  const ReviewPanel = provider.ReviewPanel;
  void version;
  return (
    <ReviewPanel
      panelId={panelId}
      presentation={presentation}
      workspaceId={workspaceId}
      taskId={taskId}
      sessionId={sessionId ?? undefined}
      reviewKey={reviewKey}
    />
  );
}

function ReviewUnavailable() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      Review unavailable.
    </div>
  );
}

function hasExplicitReviewIdentity(params: Record<string, unknown>): boolean {
  return [params.providerId, params.provider, params.reviewKey, params.prKey, params.mrKey].some(
    (value) => typeof value === "string" && value.length > 0,
  );
}

function CanonicalReviewPanel({ panelId }: { panelId: string }) {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const reviews = useNormalizedTaskReviews(taskId);
  const { selectedReview, selectReview } = useReviewItemSelection(taskId, reviews);

  if (reviews.length === 0) return <PRDetailPanelComponent panelId={panelId} />;

  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="canonical-review-panel">
      {reviews.length > 1 ? (
        <div className="shrink-0 p-2">
          <ReviewItemSelector
            reviews={reviews}
            selectedReview={selectedReview}
            onSelectReview={selectReview}
            presentation="desktop"
          />
        </div>
      ) : null}
      {selectedReview ? (
        <div className="min-h-0 flex-1">
          <ReviewDetailPanelComponent
            key={`${selectedReview.providerId}:${selectedReview.reviewKey}`}
            panelId={panelId}
            params={{
              providerId: selectedReview.providerId,
              reviewKey: selectedReview.reviewKey,
            }}
          />
        </div>
      ) : (
        <div className="flex flex-1 items-center justify-center px-6 text-center text-sm text-muted-foreground">
          Choose a review to open it.
        </div>
      )}
    </div>
  );
}

export function ReviewDetailPanelComponent({
  panelId,
  params,
  presentation = "desktop",
}: {
  panelId: string;
  params?: Record<string, unknown>;
  presentation?: "desktop" | "mobile";
}) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const hasGitHubPR = useAppStore((state) => {
    const prs = activeTaskId ? state.taskPRs.byTaskId[activeTaskId] : undefined;
    return Array.isArray(prs) && prs.length > 0;
  });
  const hasGitLabMR = useTaskMRs(activeTaskId).length > 0;
  const panelParams = params ?? {};
  const provider = resolveReviewPanelProvider(panelParams, hasGitHubPR, hasGitLabMR);
  const reviewKey = resolveReviewKey(panelParams);
  const registeredProvider = useMemo(
    () => (provider ? registry.getReviewProvider(provider) : undefined),
    [provider, registry, registryVersion],
  );

  if (panelId === "pr-detail" && !hasExplicitReviewIdentity(panelParams)) {
    return <CanonicalReviewPanel panelId={panelId} />;
  }

  if (registeredProvider && activeTaskId && workspaceId && reviewKey) {
    return (
      <RegisteredReviewPanel
        panelId={panelId}
        provider={registeredProvider}
        taskId={activeTaskId}
        workspaceId={workspaceId}
        sessionId={sessionId}
        reviewKey={reviewKey}
        presentation={presentation}
      />
    );
  }

  if (provider && provider !== "github" && provider !== "gitlab") return <ReviewUnavailable />;

  if (provider === "gitlab") {
    return <MRDetailPanelComponent panelId={panelId} params={{ mrKey: reviewKey }} />;
  }
  return <PRDetailPanelComponent panelId={panelId} params={{ prKey: reviewKey }} />;
}
