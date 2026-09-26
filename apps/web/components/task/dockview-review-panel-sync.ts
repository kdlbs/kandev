"use client";

import { useEffect, useMemo } from "react";
import type { AddPanelOptions, DockviewApi } from "dockview-react";
import { prTaskKey } from "@/components/github/pr-utils";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getPrimaryTaskPR, useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { t } from "@/lib/i18n";
import { markPRPanelOffered, wasPRPanelOffered } from "@/lib/local-storage";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import {
  CENTER_GROUP,
  isCenterCandidateGroupId,
  type LayoutState,
} from "@/lib/state/layout-manager";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { useNormalizedTaskReviews } from "./review-panel-provider";

export type CanonicalReviewParams = {
  providerId: string | undefined;
  provider: "github" | "gitlab" | undefined;
  reviewKey: string | undefined;
  connectionScope: string | undefined;
  repositoryId: string | undefined;
  changeRequestNumber: string | number | undefined;
  prKey: string | undefined;
  mrKey: string | undefined;
};

export type CanonicalReviewPanelState = {
  kind: "multiple" | "github" | "gitlab" | "registered" | "empty";
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
      kind: "multiple",
      params: {
        providerId: undefined,
        provider: undefined,
        reviewKey: undefined,
        connectionScope: undefined,
        repositoryId: undefined,
        changeRequestNumber: undefined,
        prKey: undefined,
        mrKey: undefined,
      },
      title: t("integrations:reviews", { summary: "" }),
    };
  }

  const pr = getPrimaryTaskPR(prs);
  if (pr) {
    const key = prTaskKey(pr);
    return {
      kind: "github",
      params: {
        providerId: "github",
        provider: "github",
        reviewKey: key,
        connectionScope: reviewConnectionScope(pr.pr_url, "github"),
        repositoryId: pr.repository_id || `${pr.owner}/${pr.repo}`,
        changeRequestNumber: pr.pr_number,
        prKey: key,
        mrKey: undefined,
      },
      title: t("task:pullRequest2"),
    };
  }

  const mr = mrs?.[0];
  if (mr) {
    const key = mrTaskKey(mr);
    return {
      kind: "gitlab",
      params: {
        providerId: "gitlab",
        provider: "gitlab",
        reviewKey: key,
        connectionScope: mr.host || reviewConnectionScope(mr.mr_url, "gitlab"),
        repositoryId: mr.repository_id || mr.project_path,
        changeRequestNumber: mr.mr_iid,
        prKey: undefined,
        mrKey: key,
      },
      title: t("task:mergeRequestLabel"),
    };
  }

  const registered = registeredReviews.find(
    (review) => review.providerId !== "github" && review.providerId !== "gitlab",
  );
  if (registered) {
    return {
      kind: "registered",
      params: {
        providerId: registered.providerId,
        provider: undefined,
        reviewKey: registered.reviewKey,
        connectionScope: registered.connectionScope,
        repositoryId: registered.repositoryId,
        changeRequestNumber: registered.changeRequestNumber,
        prKey: undefined,
        mrKey: undefined,
      },
      title: registered.title,
    };
  }

  return {
    kind: "empty",
    params: {
      providerId: undefined,
      provider: undefined,
      reviewKey: undefined,
      connectionScope: undefined,
      repositoryId: undefined,
      changeRequestNumber: undefined,
      prKey: undefined,
      mrKey: undefined,
    },
    title: t("common:prDetails"),
  };
}

const CANONICAL_REVIEW_PARAM_KEYS = [
  "providerId",
  "provider",
  "reviewKey",
  "connectionScope",
  "repositoryId",
  "changeRequestNumber",
  "prKey",
  "mrKey",
] as const satisfies readonly (keyof CanonicalReviewParams)[];

function hasSameReviewParams(
  current: Record<string, unknown> | undefined,
  next: CanonicalReviewParams,
): boolean {
  return CANONICAL_REVIEW_PARAM_KEYS.every((key) => current?.[key] === next[key]);
}

export type ConditionalReviewPanelAction = "add" | "remove" | "sync" | "none";

export function resolveConditionalReviewPanelAction(params: {
  hasReview: boolean;
  panelExists: boolean;
  reviewsLoaded: boolean;
  isRestoringLayout: boolean;
  isMaximized: boolean;
  wasOffered: boolean;
}): ConditionalReviewPanelAction {
  if (!params.hasReview) {
    if (params.panelExists && !params.reviewsLoaded) return "none";
    return params.panelExists ? "remove" : "none";
  }
  if (params.panelExists) return "sync";
  if (params.isRestoringLayout || params.isMaximized || params.wasOffered) return "none";
  return "add";
}

export type ConditionalReviewPanelOptions = {
  sessionId: string;
  centerGroupId: string;
  configuredPlacement?: ReviewPanelPlacement | null;
  reviewsLoaded: boolean;
  isRestoringLayout: boolean;
  isMaximized: boolean;
  wasOffered: boolean;
};

export type ReviewPanelPlacement = {
  groupId: string;
  index: number;
};

/** Resolve where a custom Default layout wants the conditional review tab. */
export function resolveConfiguredReviewPanelPlacement(
  layout: LayoutState | null,
): ReviewPanelPlacement | null {
  if (!layout) return null;
  for (const column of layout.columns) {
    for (const group of column.groups) {
      const index = group.panels.findIndex((panel) => panel.id === "pr-detail");
      if (index >= 0 && group.id) return { groupId: group.id, index };
    }
  }
  return null;
}

function resolveReviewPanelTargetGroup(
  api: DockviewApi,
  sessionId: string,
  centerGroupId: string,
): string {
  const sessionPanel = api.getPanel(`session:${sessionId}`);
  const sessionGroupId = sessionPanel?.group?.id;
  if (sessionGroupId && isCenterCandidateGroupId(sessionGroupId)) return sessionGroupId;
  return isCenterCandidateGroupId(centerGroupId) ? centerGroupId : CENTER_GROUP;
}

function resolveReviewPanelTargetPosition(
  api: DockviewApi,
  options: ConditionalReviewPanelOptions,
): NonNullable<AddPanelOptions["position"]> {
  const configured = options.configuredPlacement;
  if (configured && api.groups.some((group) => group.id === configured.groupId)) {
    return { referenceGroup: configured.groupId, index: configured.index };
  }
  return {
    referenceGroup: resolveReviewPanelTargetGroup(api, options.sessionId, options.centerGroupId),
  };
}

function addConditionalReviewPanel(
  api: DockviewApi,
  next: CanonicalReviewPanelState,
  options: ConditionalReviewPanelOptions,
): void {
  focusOrAddPanel(
    api,
    {
      id: "pr-detail",
      component: "pr-detail",
      title: next.title || t("common:prDetails"),
      position: resolveReviewPanelTargetPosition(api, options),
      params: next.params,
    },
    true,
  );
  markPRPanelOffered(options.sessionId);
}

function syncExistingReviewPanel(
  panel: NonNullable<ReturnType<DockviewApi["getPanel"]>>,
  next: CanonicalReviewPanelState,
  options: ConditionalReviewPanelOptions,
): boolean {
  if (hasCanonicalReview(next)) markPRPanelOffered(options.sessionId);
  const paramsChanged = !hasSameReviewParams(panel.params, next.params);
  const titleChanged = panel.api.title !== next.title;
  if (!paramsChanged && !titleChanged) return false;
  if (paramsChanged) panel.api.updateParameters(next.params);
  if (titleChanged) panel.api.setTitle(next.title);
  return true;
}

function hasCanonicalReview(next: CanonicalReviewPanelState): boolean {
  return next.kind !== "empty";
}

function resolveReviewPanelAction(
  panel: ReturnType<DockviewApi["getPanel"]>,
  next: CanonicalReviewPanelState,
  options: ConditionalReviewPanelOptions,
): ConditionalReviewPanelAction {
  return resolveConditionalReviewPanelAction({
    hasReview: hasCanonicalReview(next),
    panelExists: !!panel,
    reviewsLoaded: options.reviewsLoaded,
    isRestoringLayout: options.isRestoringLayout,
    isMaximized: options.isMaximized,
    wasOffered: options.wasOffered,
  });
}

/**
 * Synchronize the canonical PR Details panel and manage the conditional panel
 * shown for a linked review.
 *
 * Review association owns panel existence. A custom Default layout can still
 * provide the group and tab index used when a linked review makes it visible.
 */
export function syncCanonicalReviewPanel(
  api: DockviewApi,
  next: CanonicalReviewPanelState,
  options: ConditionalReviewPanelOptions,
): boolean {
  const panel = api.getPanel("pr-detail");
  const action = resolveReviewPanelAction(panel, next, options);

  if (action === "remove") {
    panel?.api.close();
    return true;
  }

  if (action === "add") {
    addConditionalReviewPanel(api, next, options);
    return true;
  }

  return action === "sync" && panel ? syncExistingReviewPanel(panel, next, options) : false;
}

function reviewIdentity(state: CanonicalReviewPanelState): string {
  return [
    state.kind,
    state.params.providerId ?? "none",
    state.params.connectionScope ?? "",
    state.params.repositoryId ?? "",
    String(state.params.changeRequestNumber ?? ""),
  ].join(":");
}

function reviewConnectionScope(url: string, fallback: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return fallback;
  }
}

/** Keep an existing canonical PR Details panel in sync with the active task. */
export function useSyncReviewPanel() {
  const appStore = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const { loaded: githubReviewsLoaded } = useTaskPR(taskId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const reviews = useNormalizedTaskReviews(taskId);
  const registeredReviews = useMemo(
    () =>
      reviews.filter((review) => review.providerId !== "github" && review.providerId !== "gitlab"),
    [reviews],
  );
  const gitlabReviewsLoaded = useAppStore((state) =>
    workspaceId ? Object.hasOwn(state.taskMRs.byWorkspaceId, workspaceId) : false,
  );
  const reviewsLoaded = githubReviewsLoaded && gitlabReviewsLoaded;
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
  const isRestoringLayout = useDockviewStore((state) => state.isRestoringLayout);
  const isMaximized = useDockviewStore((state) => state.preMaximizeLayout !== null);
  const centerGroupId = useDockviewStore((state) => state.centerGroupId);
  const userDefaultLayout = useDockviewStore((state) => state.userDefaultLayout);

  useEffect(() => {
    if (!taskId || !sessionId || !workspaceId || !hasApi) return;

    let innerFrame: number | null = null;
    const outerFrame = requestAnimationFrame(() => {
      innerFrame = requestAnimationFrame(() => {
        const live = appStore.getState();
        if (
          live.tasks.activeTaskId !== taskId ||
          live.tasks.activeSessionId !== sessionId ||
          live.workspaces.activeId !== workspaceId
        )
          return;

        const api = useDockviewStore.getState().api;
        if (!api) return;
        const dockview = useDockviewStore.getState();
        syncCanonicalReviewPanel(
          api,
          resolveCanonicalReviewPanelState(
            live.taskPRs.byTaskId[taskId],
            live.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
            registeredReviews,
          ),
          {
            sessionId,
            centerGroupId: dockview.centerGroupId,
            configuredPlacement: resolveConfiguredReviewPanelPlacement(dockview.userDefaultLayout),
            reviewsLoaded,
            isRestoringLayout: dockview.isRestoringLayout,
            isMaximized: dockview.preMaximizeLayout !== null,
            wasOffered: wasPRPanelOffered(sessionId),
          },
        );
      });
    });

    return () => {
      cancelAnimationFrame(outerFrame);
      if (innerFrame !== null) cancelAnimationFrame(innerFrame);
    };
  }, [
    appStore,
    centerGroupId,
    hasApi,
    identity,
    isMaximized,
    isRestoringLayout,
    reviewsLoaded,
    registeredReviews,
    sessionId,
    taskId,
    userDefaultLayout,
    workspaceId,
  ]);
}
