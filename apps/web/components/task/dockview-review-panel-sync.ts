"use client";

import { useEffect } from "react";
import type { DockviewApi } from "dockview-react";
import { prTaskKey } from "@/components/github/pr-utils";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getPrimaryTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useDockviewStore } from "@/lib/state/dockview-store";
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

/**
 * Update only the review identity of a layout-owned PR Details panel.
 *
 * Layout profile and task-layout restoration own panel existence and position.
 * This helper deliberately never calls add, close, move, or activate APIs.
 */
export function syncCanonicalReviewPanel(api: DockviewApi, next: CanonicalReviewParams): boolean {
  const panel = api.getPanel("pr-detail");
  if (!panel || hasSameReviewParams(panel.params, next)) return false;
  panel.api.updateParameters(next);
  return true;
}

function reviewIdentity(params: CanonicalReviewParams): string {
  return `${params.provider ?? "none"}:${params.prKey ?? ""}:${params.mrKey ?? ""}`;
}

/** Keep an existing canonical PR Details panel in sync with the active task. */
export function useSyncReviewPanel() {
  const appStore = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
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
          resolveCanonicalReviewParams(
            live.taskPRs.byTaskId[taskId],
            live.taskMRs.byWorkspaceId[workspaceId]?.[taskId],
          ),
        );
      });
    });

    return () => {
      cancelAnimationFrame(outerFrame);
      if (innerFrame !== null) cancelAnimationFrame(innerFrame);
    };
  }, [appStore, hasApi, identity, taskId, workspaceId]);
}
