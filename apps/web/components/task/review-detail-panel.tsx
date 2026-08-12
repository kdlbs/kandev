"use client";

import { useMemo } from "react";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { MRDetailPanelComponent } from "@/components/gitlab/mr-detail-panel";
import { useAppStore } from "@/components/state-provider";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { t } from "@/lib/i18n";
import type { PluginReviewProviderRegistration } from "@/lib/plugins/registry";
import {
  resolveReviewKey,
  resolveReviewPanelProvider,
  useNormalizedTaskReviews,
  useReviewProviderUpdates,
} from "./review-panel-provider";
import { ReviewItemSelector } from "./review-item-selector";
import { reviewItemId, useReviewItemSelection } from "./review-selection";

type RegisteredReviewPanelProps = {
  panelId: string;
  provider: PluginReviewProviderRegistration;
  taskId: string;
  workspaceId: string;
  sessionId: string | null;
  reviewKey: string;
  connectionScope: string;
  repositoryId: string;
  changeRequestNumber: string | number;
  presentation: "desktop" | "mobile";
};

function RegisteredReviewPanel({
  panelId,
  provider,
  taskId,
  workspaceId,
  sessionId,
  reviewKey,
  connectionScope,
  repositoryId,
  changeRequestNumber,
  presentation,
}: RegisteredReviewPanelProps) {
  const providers = useMemo(() => [provider], [provider]);
  const version = useReviewProviderUpdates(taskId, providers);
  const items = provider.getSnapshot(taskId);
  const selected = items.find(
    (item) =>
      item.repositoryId === repositoryId &&
      item.connectionScope === connectionScope &&
      String(item.changeRequestNumber) === String(changeRequestNumber),
  );

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
      connectionScope={connectionScope}
      repositoryId={repositoryId}
      changeRequestNumber={changeRequestNumber}
    />
  );
}

function ReviewUnavailable() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      {t("integrations:reviewUnavailable")}
    </div>
  );
}

function hasExplicitReviewIdentity(params: Record<string, unknown>): boolean {
  return [params.providerId, params.provider, params.reviewKey, params.prKey, params.mrKey].some(
    (value) => typeof value === "string" && value.length > 0,
  );
}

function parseRegisteredReviewIdentity(params: Record<string, unknown>) {
  const reviewKey = resolveReviewKey(params);
  const repositoryId = typeof params.repositoryId === "string" ? params.repositoryId : undefined;
  const connectionScope =
    typeof params.connectionScope === "string" ? params.connectionScope : undefined;
  const changeRequestNumber =
    typeof params.changeRequestNumber === "string" || typeof params.changeRequestNumber === "number"
      ? params.changeRequestNumber
      : undefined;
  return { reviewKey, repositoryId, connectionScope, changeRequestNumber };
}

function resolveRegisteredReviewPanelProps({
  panelId,
  provider,
  taskId,
  workspaceId,
  sessionId,
  identity,
  presentation,
}: {
  panelId: string;
  provider: PluginReviewProviderRegistration | undefined;
  taskId: string | null;
  workspaceId: string | null;
  sessionId: string | null;
  identity: ReturnType<typeof parseRegisteredReviewIdentity>;
  presentation: "desktop" | "mobile";
}): RegisteredReviewPanelProps | null {
  if (
    !provider ||
    !taskId ||
    !workspaceId ||
    !identity.reviewKey ||
    !identity.connectionScope ||
    !identity.repositoryId ||
    identity.changeRequestNumber === undefined
  ) {
    return null;
  }
  return {
    panelId,
    provider,
    taskId,
    workspaceId,
    sessionId,
    reviewKey: identity.reviewKey,
    connectionScope: identity.connectionScope,
    repositoryId: identity.repositoryId,
    changeRequestNumber: identity.changeRequestNumber,
    presentation,
  };
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
            key={reviewItemId(selectedReview)}
            panelId={panelId}
            params={{
              providerId: selectedReview.providerId,
              reviewKey: selectedReview.reviewKey,
              connectionScope: selectedReview.connectionScope,
              repositoryId: selectedReview.repositoryId,
              changeRequestNumber: selectedReview.changeRequestNumber,
            }}
          />
        </div>
      ) : (
        <div className="flex flex-1 items-center justify-center px-6 text-center text-sm text-muted-foreground">
          {t("integrations:chooseReviewToOpen")}
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
  const identity = parseRegisteredReviewIdentity(panelParams);
  const registeredProvider = useMemo(
    () => (provider ? registry.getReviewProvider(provider) : undefined),
    [provider, registry, registryVersion],
  );
  const registeredPanelProps = resolveRegisteredReviewPanelProps({
    panelId,
    provider: registeredProvider,
    taskId: activeTaskId,
    workspaceId,
    sessionId,
    identity,
    presentation,
  });

  if (panelId === "pr-detail" && !hasExplicitReviewIdentity(panelParams)) {
    return <CanonicalReviewPanel panelId={panelId} />;
  }

  if (registeredPanelProps) return <RegisteredReviewPanel {...registeredPanelProps} />;

  if (provider && provider !== "github" && provider !== "gitlab") return <ReviewUnavailable />;

  if (provider === "gitlab") {
    return <MRDetailPanelComponent panelId={panelId} params={{ mrKey: identity.reviewKey }} />;
  }
  return <PRDetailPanelComponent panelId={panelId} params={{ prKey: identity.reviewKey }} />;
}
