"use client";

import { useCallback, useEffect } from "react";
import { DockviewReact, type DockviewReadyEvent } from "dockview-react";
import { themeKandev } from "@/lib/layout/dockview-theme";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { applyLayout } from "@/lib/state/layout-manager";
import { setupGroupTracking, setupPortalCleanup } from "@/components/task/dockview-layout-setup";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  resolveTaskPullRequestProps,
  selectWorkspaceRepositories,
} from "@/components/task/task-page-content-helpers";
import {
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  panel,
} from "@/lib/state/layout-manager/constants";
import type { LayoutState } from "@/lib/state/layout-manager/types";

import { LeftHeaderActions, RightHeaderActions } from "@/components/task/dockview-header-actions";
import { DockviewWatermark } from "@/components/task/dockview-watermark";

import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { ensureTaskSession } from "@/lib/services/session-launch-service";

import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { PanelPortalHost } from "@/lib/layout/panel-portal-host";
import {
  dockviewComponents,
  dockviewTabComponents,
  ContextMenuTab,
  renderPanel,
} from "@/components/task/dockview-shared";
import type { Task as OfficeTask } from "./types";

// ---------------------------------------------------------------------------
// OFFICE LAYOUT — no sidebar, no persistence, no multi-session
// ---------------------------------------------------------------------------

function officeLayout(): LayoutState {
  return {
    columns: [
      {
        id: "center",
        groups: [{ id: CENTER_GROUP, panels: [panel("chat")] }],
      },
      {
        id: "right",
        pinned: true,
        width: 350,
        groups: [
          { id: RIGHT_TOP_GROUP, panels: [panel("files"), panel("changes")] },
          { id: RIGHT_BOTTOM_GROUP, panels: [panel("terminal-default")] },
        ],
      },
    ],
  };
}

type OfficeDockviewLayoutProps = {
  taskId: string;
  sessionId: string | null;
  task?: OfficeTask;
};

export function OfficeDockviewLayout({ taskId, sessionId, task }: OfficeDockviewLayoutProps) {
  const setApi = useDockviewStore((s) => s.setApi);
  const appStore = useAppStoreApi();
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const storedOfficeTask = useAppStore((s) =>
    s.office.tasks.items.find((item) => item.id === taskId),
  );
  const officeTask = task ?? storedOfficeTask;
  const workspaceRepositories = useAppStore((s) =>
    selectWorkspaceRepositories(s.repositories.itemsByWorkspaceId, officeTask?.workspaceId),
  );
  const pullRequestProps = resolveTaskPullRequestProps(officeTask ?? null, workspaceRepositories);

  // Wire office session into the global store so shared panels can read it.
  useEffect(() => {
    if (taskId && sessionId) {
      setActiveSession(taskId, sessionId);
    } else if (taskId) {
      setActiveTask(taskId);
    }
  }, [taskId, sessionId, setActiveSession, setActiveTask]);

  const setAgentctlStatus = useAppStore((s) => s.setSessionAgentctlStatus);
  const setTaskSession = useAppStore((s) => s.setTaskSession);

  // Ensure the execution (agentctl) is running so file/terminal/changes panels work.
  // Office tasks are one-off — the execution may have been torn down after completion.
  // After ensure resolves, mark agentctl as ready — the WS event may have fired
  // before this page subscribed to the session.
  useEffect(() => {
    if (!taskId) return;
    ensureTaskSession(taskId, { ensureExecution: true })
      .then((resp) => {
        if (resp.session_id) {
          setAgentctlStatus(resp.session_id, {
            status: "ready",
            updatedAt: new Date().toISOString(),
          });
          // Populate workspace_path for quick-chat sessions so the file
          // browser and chat links use the effective task root.
          if (resp.workspace_path) {
            const existing = appStore.getState().taskSessions.items[resp.session_id];
            if (existing && !existing.workspace_path) {
              setTaskSession({ ...existing, workspace_path: resp.workspace_path });
            }
          }
        }
      })
      .catch(() => {
        // Non-fatal: panels will show appropriate empty/retry states.
      });
  }, [taskId, setAgentctlStatus, setTaskSession, appStore]);

  // Clean up on unmount — release portals and clear active session.
  useEffect(() => {
    return () => {
      panelPortalManager.releaseAll();
    };
  }, []);

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      const api = event.api;
      setApi(api);

      const pinnedWidths = new Map<string, number>();
      const ids = applyLayout(api, officeLayout(), pinnedWidths);

      useDockviewStore.setState({
        ...ids,
        currentLayoutEnvId: null,
        sidebarVisible: false,
        rightPanelsVisible: true,
        pinnedWidths,
      });

      setupGroupTracking(api);
      setupPortalCleanup(api, appStore);
    },
    [setApi, appStore],
  );

  return (
    <VcsDialogsProvider
      sessionId={sessionId}
      baseBranch={pullRequestProps.baseBranch}
      pullRequestBaseBranch={pullRequestProps.pullRequestTarget}
      pullRequestTargetsByRepository={pullRequestProps.pullRequestTargetsByRepository}
      taskTitle={pullRequestProps.taskTitle}
      displayBranch={officeTask?.repositories?.[0]?.checkout_branch}
    >
      <div className="flex-1 min-h-0">
        <DockviewReact
          theme={themeKandev}
          components={dockviewComponents}
          tabComponents={dockviewTabComponents}
          defaultTabComponent={ContextMenuTab}
          leftHeaderActionsComponent={LeftHeaderActions}
          rightHeaderActionsComponent={RightHeaderActions}
          watermarkComponent={DockviewWatermark}
          onReady={onReady}
          defaultRenderer="always"
          className="h-full"
        />
        <PanelPortalHost renderPanel={renderPanel} />
      </div>
    </VcsDialogsProvider>
  );
}
