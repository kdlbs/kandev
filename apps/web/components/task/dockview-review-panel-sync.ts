"use client";

import { useEffect, useMemo } from "react";
import type { DockviewApi } from "dockview-react";
import { prTaskKey } from "@/components/github/pr-utils";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getPrimaryTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { useNormalizedTaskReviews } from "./review-panel-provider";

export type CanonicalReviewParams = {
  providerId: string | undefined;
  provider: "github" | "gitlab" | undefined;
  reviewKey: string | undefined;
  prKey: string | undefined;
  mrKey: string | undefined;
};

export type CanonicalReviewPanelState = {
  params: CanonicalReviewParams;
  title: string;
};

export function resolveCanonicalReviewPanelState(
  prs: TaskPR[] | undefined,
  mrs: TaskMR[] | undefined,
  registeredReviews: readonly ReviewItemSummary[] = [],
): CanonicalReviewPanelState {
  const linkedProviderIds = new Set<string>();
  if (prs?.length) linkedProviderIds.add("github");
  if (mrs?.length) linkedProviderIds.add("gitlab");
  registeredReviews.forEach((review) => linkedProviderIds.add(review.providerId));
  if (linkedProviderIds.size > 1) {
    return {
      params: {
        providerId: undefined,
        provider: undefined,
        reviewKey: undefined,
        prKey: undefined,
        mrKey: undefined,
      },
      title: "Reviews",
    };
  }

  const pr = getPrimaryTaskPR(prs);
  if (pr) {
    const key = prTaskKey(pr);
    return {
      params: {
        providerId: "github",
        provider: "github",
        reviewKey: key,
        prKey: key,
        mrKey: undefined,
      },
      title: "Pull Request",
    };
  }

  const mr = mrs?.[0];
  if (mr) {
    const key = mrTaskKey(mr);
    return {
      params: {
        providerId: "gitlab",
        provider: "gitlab",
        reviewKey: key,
        prKey: undefined,
        mrKey: key,
      },
      title: "Merge Request",
    };
  }

  const registered = registeredReviews.find(
    (review) => review.providerId !== "github" && review.providerId !== "gitlab",
  );
  if (registered) {
    return {
      params: {
        providerId: registered.providerId,
        provider: undefined,
        reviewKey: registered.reviewKey,
        prKey: undefined,
        mrKey: undefined,
      },
      title: registered.title,
    };
  }

  return {
    params: {
      providerId: undefined,
      provider: undefined,
      reviewKey: undefined,
      prKey: undefined,
      mrKey: undefined,
    },
    title: "PR Details",
  };
}

function hasSameReviewParams(
  current: Record<string, unknown> | undefined,
  next: CanonicalReviewParams,
): boolean {
  return (
    current?.providerId === next.providerId &&
    current?.provider === next.provider &&
    current?.reviewKey === next.reviewKey &&
    current?.prKey === next.prKey &&
    current?.mrKey === next.mrKey
  );
}

/**
 * Update the review identity and title of a layout-owned PR Details panel.
 *
 * Layout profile and task-layout restoration own panel existence and position.
 * This helper deliberately never calls add, close, move, or activate APIs.
 */
export function syncCanonicalReviewPanel(
  api: DockviewApi,
  next: CanonicalReviewPanelState,
): boolean {
  const panel = api.getPanel("pr-detail");
  if (!panel) return false;
  const paramsChanged = !hasSameReviewParams(panel.params, next.params);
  const titleChanged = panel.api.title !== next.title;
  if (!paramsChanged && !titleChanged) return false;
  if (paramsChanged) panel.api.updateParameters(next.params);
  if (titleChanged) panel.api.setTitle(next.title);
  return true;
}

function reviewIdentity(state: CanonicalReviewPanelState): string {
  return `${state.params.providerId ?? "none"}:${state.params.reviewKey ?? ""}:${state.title}`;
}

/** Keep an existing canonical PR Details panel in sync with the active task. */
export function useSyncReviewPanel() {
  const appStore = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const reviews = useNormalizedTaskReviews(taskId);
  const registeredReviews = useMemo(
    () =>
      reviews.filter((review) => review.providerId !== "github" && review.providerId !== "gitlab"),
    [reviews],
  );
  const identity = useAppStore((state) => {
    if (!taskId || !workspaceId) return "none";
    return reviewIdentity(
      resolveCanonicalReviewPanelState(
        state.taskPRs.byTaskId[taskId],
        state.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
        registeredReviews,
      ),
    );
  });
  const hasApi = useDockviewStore((state) => !!state.api);

  useEffect(() => {
    if (!taskId || !workspaceId || !hasApi) return;

    let innerFrame: number | null = null;
    const outerFrame = requestAnimationFrame(() => {
      innerFrame = requestAnimationFrame(() => {
        const live = appStore.getState();
        if (live.tasks.activeTaskId !== taskId || live.workspaces.activeId !== workspaceId) return;

        const api = useDockviewStore.getState().api;
        if (!api) return;
        syncCanonicalReviewPanel(
          api,
          resolveCanonicalReviewPanelState(
            live.taskPRs.byTaskId[taskId],
            live.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
            registeredReviews,
          ),
        );
      });
    });

    return () => {
      cancelAnimationFrame(outerFrame);
      if (innerFrame !== null) cancelAnimationFrame(innerFrame);
    };
  }, [appStore, hasApi, identity, registeredReviews, taskId, workspaceId]);
}
