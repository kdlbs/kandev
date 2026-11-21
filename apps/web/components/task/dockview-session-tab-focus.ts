import type { MutableRefObject } from "react";
import type { DockviewApi } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { CENTER_GROUP, isCenterCandidateGroupId } from "@/lib/state/layout-manager";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { hasTaskRouteFocus, consumeTaskRouteFocus } from "@/lib/state/task-navigation-focus";
import {
  shouldActivateSessionPanel,
  restorePreservedActivePanel,
} from "./dockview-session-tab-activation";
import {
  getRemovalSurfaceFocus,
  consumeRemovalSurfaceFocus,
} from "./dockview-session-removal-focus";

const debug = createDebugLogger("dockview:session-tabs");

export type AutoSessionTabRefs = {
  sessionTabCreatedRef: MutableRefObject<Set<string>>;
  prevTaskIdRef: MutableRefObject<string | null>;
  prevSessionIdRef: MutableRefObject<string | null>;
};

function applySessionPanelActivation({
  api,
  appStore,
  panel,
  workflowFocusRequestId,
  removalFocus,
  activate,
}: {
  api: DockviewApi;
  appStore: StoreApi<AppState>;
  panel: NonNullable<ReturnType<DockviewApi["getPanel"]>>;
  workflowFocusRequestId: number | null;
  removalFocus: ReturnType<typeof getRemovalSurfaceFocus>;
  activate: boolean;
}): void {
  if (activate) {
    panel.api.setActive();
    if (workflowFocusRequestId !== null) {
      appStore.getState().acknowledgeWorkflowSessionFocus(workflowFocusRequestId);
    }
    if (removalFocus && !removalFocus.cancelled) {
      restorePreservedActivePanel(api, removalFocus.restorePanelId, true);
    }
  }
  if (removalFocus) consumeRemovalSurfaceFocus(api, removalFocus.groupId);
  useDockviewStore.setState({
    centerGroupId: isCenterCandidateGroupId(panel.group.id) ? panel.group.id : CENTER_GROUP,
  });
}

/**
 * Activate the newly-ensured session panel and update the center-group store
 * entry. Returns the resolved active panel for sibling anchoring.
 */
export function activateSessionPanel({
  api,
  effectiveSessionId,
  activation,
  refs,
  tid,
  appStore,
  workflowFocusRequestId,
}: {
  api: DockviewApi;
  effectiveSessionId: string;
  activation: {
    sessionPanelExistedBefore: boolean;
    activePanelIdBeforeEnsure: string | null;
  };
  refs: AutoSessionTabRefs;
  tid: string | null;
  appStore: StoreApi<AppState>;
  workflowFocusRequestId: number | null;
}): ReturnType<DockviewApi["getPanel"]> {
  const activePanel = api.getPanel(`session:${effectiveSessionId}`);
  if (!activePanel) return activePanel;
  const removalFocus = tid
    ? getRemovalSurfaceFocus(api, appStore, tid, effectiveSessionId)
    : undefined;
  const removalRequested = Boolean(removalFocus && !removalFocus.cancelled);
  const routeRequested = hasTaskRouteFocus(appStore, tid, effectiveSessionId);

  const activePanelIdAfterReorder = api.activePanel?.id ?? null;
  const shouldActivate =
    permitsRequestedFocus(removalFocus, workflowFocusRequestId, routeRequested) &&
    shouldActivateSessionPanel({
      sessionPanelExistedBefore: activation.sessionPanelExistedBefore,
      prevTaskId: refs.prevTaskIdRef.current,
      prevSessionId: refs.prevSessionIdRef.current,
      currentTaskId: tid,
      currentSessionId: effectiveSessionId,
      currentActivePanelId: activation.activePanelIdBeforeEnsure,
      focusRequested: workflowFocusRequestId !== null || removalRequested || routeRequested,
    });
  if (isDebug()) {
    debug("useAutoSessionTab: activation decision", {
      effectiveSessionId,
      shouldActivate,
      sessionPanelExistedBefore: activation.sessionPanelExistedBefore,
      prevTaskId: refs.prevTaskIdRef.current,
      prevSessionId: refs.prevSessionIdRef.current,
      currentTaskId: tid,
      activePanelIdBeforeEnsure: activation.activePanelIdBeforeEnsure,
      activePanelIdAfterReorder,
      activeGroupId: activePanel.group.id,
    });
  }
  const restoreSelectedSession =
    activation.activePanelIdBeforeEnsure === activePanel.id &&
    activePanelIdAfterReorder !== activePanel.id;
  applySessionPanelActivation({
    api,
    appStore,
    panel: activePanel,
    workflowFocusRequestId,
    removalFocus,
    activate: shouldActivate || restoreSelectedSession,
  });
  consumeVisibleRouteFocus(appStore, tid, activePanel, routeRequested);
  return activePanel;
}

function permitsRequestedFocus(
  removal: ReturnType<typeof getRemovalSurfaceFocus>,
  workflow: number | null,
  route: boolean,
) {
  return !removal?.cancelled || workflow !== null || route;
}

function consumeVisibleRouteFocus(
  store: StoreApi<AppState>,
  taskId: string | null,
  panel: NonNullable<ReturnType<DockviewApi["getPanel"]>>,
  requested: boolean,
) {
  if (requested && panel.group.activePanel?.id === panel.id)
    consumeTaskRouteFocus(
      store,
      taskId,
      panel.id.slice("session:".length),
      useDockviewStore.getState().currentLayoutEnvId,
    );
}
