import { useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { markPRPanelOffered, wasPRPanelOffered } from "@/lib/local-storage";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { AppState } from "@/lib/state/store";
import type { TaskMR } from "@/lib/types/gitlab";
import { resolvePRPanelTargetGroup } from "./dockview-session-tabs";

export function resolveAutoMRPanelAction(params: {
  hasMR: boolean;
  panelExists: boolean;
  restoring: boolean;
  maximized: boolean;
  offered: boolean;
}): "add" | "remove" | "none" {
  if (!params.hasMR) return params.panelExists ? "remove" : "none";
  if (params.panelExists || params.restoring || params.maximized || params.offered) return "none";
  return "add";
}

type AutoMRPanelIdentity = {
  taskId: string | null;
  sessionId: string | null;
  workspaceId: string | null;
};

export function isLiveAutoMRPanelIdentity(
  expected: AutoMRPanelIdentity,
  live: AutoMRPanelIdentity,
): boolean {
  return (
    expected.taskId === live.taskId &&
    expected.sessionId === live.sessionId &&
    expected.workspaceId === live.workspaceId
  );
}

export function scheduleAfterTwoFrames(
  run: FrameRequestCallback,
  schedule: typeof requestAnimationFrame = requestAnimationFrame,
  cancel: typeof cancelAnimationFrame = cancelAnimationFrame,
): () => void {
  let innerFrame: number | null = null;
  const outerFrame = schedule((time) => {
    innerFrame = schedule(run);
    void time;
  });
  return () => {
    cancel(outerFrame);
    if (innerFrame !== null) cancel(innerFrame);
  };
}

function livePrimaryMR(app: AppState, expected: AutoMRPanelIdentity): TaskMR | null | undefined {
  const live = {
    taskId: app.tasks.activeTaskId,
    sessionId: app.tasks.activeSessionId,
    workspaceId: app.workspaces.activeId,
  };
  if (!isLiveAutoMRPanelIdentity(expected, live) || !live.workspaceId || !live.taskId)
    return undefined;
  if ((app.taskPRs.byTaskId[live.taskId]?.length ?? 0) > 0) return null;
  return app.taskMRs.byWorkspaceId[live.workspaceId]?.[live.taskId]?.[0] ?? null;
}

function applyAutoMRPanel(expected: AutoMRPanelIdentity, app: AppState) {
  const liveMR = livePrimaryMR(app, expected);
  if (liveMR === undefined) return;
  const api = useDockviewStore.getState().api;
  if (!api || !expected.sessionId) return;
  const panel = api.getPanel("mr-detail");
  const dockview = useDockviewStore.getState();
  const action = resolveAutoMRPanelAction({
    hasMR: !!liveMR,
    panelExists: !!panel,
    restoring: dockview.isRestoringLayout,
    maximized: dockview.preMaximizeLayout !== null,
    offered: wasPRPanelOffered(expected.sessionId),
  });
  if (action === "remove") return panel?.api.close();
  if (!liveMR) return;
  const key = mrTaskKey(liveMR);
  if (panel) {
    if (panel.params?.mrKey !== key) panel.api.updateParameters({ mrKey: key });
  } else if (action === "add") {
    focusOrAddPanel(api, {
      id: "mr-detail",
      component: "mr-detail",
      title: "Merge Request",
      position: {
        referenceGroup: resolvePRPanelTargetGroup(api, expected.sessionId, dockview.centerGroupId),
      },
      inactive: true,
      params: { mrKey: key },
    });
  } else {
    return;
  }
  markPRPanelOffered(expected.sessionId);
}

/** Auto-show a GitLab MR panel when the task has no GitHub PR panel to prefer. */
export function useAutoMRPanel() {
  const appStore = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const primaryMR = useAppStore((state) => {
    if (!taskId || !workspaceId) return null;
    const hasGitHub = (state.taskPRs.byTaskId[taskId]?.length ?? 0) > 0;
    if (hasGitHub) return null;
    return state.taskMRs.byWorkspaceId[workspaceId]?.[taskId]?.[0] ?? null;
  });
  const hasApi = useDockviewStore((state) => !!state.api);

  useEffect(() => {
    if (!taskId || !sessionId || !hasApi) return;
    const expected = { taskId, sessionId, workspaceId };
    return scheduleAfterTwoFrames(() => applyAutoMRPanel(expected, appStore.getState()));
  }, [appStore, taskId, sessionId, workspaceId, hasApi, primaryMR]);
}
