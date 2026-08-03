"use client";

import { useEffect } from "react";
import type { DockviewApi } from "dockview-react";
import { prTaskKey } from "@/components/github/pr-utils";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getPrimaryTaskPR } from "@/hooks/domains/github/use-task-pr";
import { markPRPanelOffered, wasPRPanelOffered } from "@/lib/local-storage";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { CENTER_GROUP, isCenterCandidateGroupId } from "@/lib/state/layout-manager";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

export type CanonicalReviewParams = {
  provider: "github" | "gitlab" | undefined;
  prKey: string | undefined;
  mrKey: string | undefined;
};

export function resolveCanonicalReviewParams(
  prs: TaskPR[] | undefined,
  mrs: TaskMR[] | undefined,
): CanonicalReviewParams {
  const pr = getPrimaryTaskPR(prs);
  if (pr) return { provider: "github", prKey: prTaskKey(pr), mrKey: undefined };

  const mr = mrs?.[0];
  if (mr) return { provider: "gitlab", prKey: undefined, mrKey: mrTaskKey(mr) };

  return { provider: undefined, prKey: undefined, mrKey: undefined };
}

function hasSameReviewParams(
  current: Record<string, unknown> | undefined,
  next: CanonicalReviewParams,
): boolean {
  return (
    current?.provider === next.provider &&
    current?.prKey === next.prKey &&
    current?.mrKey === next.mrKey
  );
}

export type ConditionalReviewPanelAction = "add" | "remove" | "sync" | "none";

export function resolveConditionalReviewPanelAction(params: {
  hasReview: boolean;
  panelExists: boolean;
  autoAddedForReview: boolean;
  isRestoringLayout: boolean;
  isMaximized: boolean;
  wasOffered: boolean;
}): ConditionalReviewPanelAction {
  if (!params.hasReview) {
    if (params.autoAddedForReview) return "remove";
    return params.panelExists ? "sync" : "none";
  }
  if (params.panelExists) return "sync";
  if (params.isRestoringLayout || params.isMaximized || params.wasOffered) return "none";
  return "add";
}

type ConditionalReviewPanelOptions = {
  sessionId: string;
  centerGroupId: string;
  isRestoringLayout: boolean;
  isMaximized: boolean;
  wasOffered: boolean;
};

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

function addConditionalReviewPanel(
  api: DockviewApi,
  next: CanonicalReviewParams,
  options: ConditionalReviewPanelOptions,
): void {
  focusOrAddPanel(
    api,
    {
      id: "pr-detail",
      component: "pr-detail",
      title: "PR Details",
      position: {
        referenceGroup: resolveReviewPanelTargetGroup(
          api,
          options.sessionId,
          options.centerGroupId,
        ),
      },
      params: { ...next, autoAddedForReview: true },
    },
    true,
  );
  markPRPanelOffered(options.sessionId);
}

function syncExistingReviewPanel(
  panel: NonNullable<ReturnType<DockviewApi["getPanel"]>>,
  next: CanonicalReviewParams,
  options?: ConditionalReviewPanelOptions,
): boolean {
  if (next.provider && options) markPRPanelOffered(options.sessionId);
  if (hasSameReviewParams(panel.params, next)) return false;
  panel.api.updateParameters(next);
  return true;
}

/**
 * Synchronize the canonical PR Details panel and, when options are supplied,
 * manage the conditional panel shown for a linked review.
 *
 * Layout profile and task-layout restoration own explicit panel existence and
 * position. Conditional panels are marked in their params so review loss only
 * removes panels created by this path.
 */
export function syncCanonicalReviewPanel(
  api: DockviewApi,
  next: CanonicalReviewParams,
  options?: ConditionalReviewPanelOptions,
): boolean {
  const panel = api.getPanel("pr-detail");
  const action = resolveConditionalReviewPanelAction({
    hasReview: !!next.provider,
    panelExists: !!panel,
    autoAddedForReview: panel?.params?.autoAddedForReview === true,
    isRestoringLayout: options?.isRestoringLayout ?? false,
    isMaximized: options?.isMaximized ?? false,
    wasOffered: options?.wasOffered ?? false,
  });

  if (action === "remove") {
    panel?.api.close();
    return true;
  }

  if (action === "add" && options) {
    addConditionalReviewPanel(api, next, options);
    return true;
  }

  return action === "sync" && panel ? syncExistingReviewPanel(panel, next, options) : false;
}

function reviewIdentity(params: CanonicalReviewParams): string {
  return `${params.provider ?? "none"}:${params.prKey ?? ""}:${params.mrKey ?? ""}`;
}

/** Keep an existing canonical PR Details panel in sync with the active task. */
export function useSyncReviewPanel() {
  const appStore = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const identity = useAppStore((state) => {
    if (!taskId || !workspaceId) return "none";
    return reviewIdentity(
      resolveCanonicalReviewParams(
        state.taskPRs.byTaskId[taskId],
        state.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
      ),
    );
  });
  const hasApi = useDockviewStore((state) => !!state.api);
  const isRestoringLayout = useDockviewStore((state) => state.isRestoringLayout);
  const isMaximized = useDockviewStore((state) => state.preMaximizeLayout !== null);
  const centerGroupId = useDockviewStore((state) => state.centerGroupId);

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
          resolveCanonicalReviewParams(
            live.taskPRs.byTaskId[taskId],
            live.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
          ),
          {
            sessionId,
            centerGroupId: dockview.centerGroupId,
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
    sessionId,
    taskId,
    workspaceId,
  ]);
}
