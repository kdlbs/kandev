import type { DockviewApi } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { isConfirmedSessionRemoved } from "@/lib/state/session-removal";
import { getProvenRemovalPredecessor } from "@/lib/state/session-removal-handoff";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { shouldPreserveActivePanel } from "./dockview-session-tab-activation";

type SurfaceOwner = {
  store: StoreApi<AppState>;
  taskId: string;
  previousSessionId: string;
  groupId: string;
  navigationRevision: number;
  workflowFocus: AppState["workflowSessionFocus"];
  scopeIdentity: string;
  restorePanelId: string | null;
  cancelled: boolean;
  focusDelivered: boolean;
};
const surfaces = new WeakMap<DockviewApi, Map<string, SurfaceOwner>>();

function captureVisiblePredecessor(
  api: DockviewApi,
  store: StoreApi<AppState>,
  current: AppState,
  previous: AppState,
): SurfaceOwner | undefined {
  const taskId = previous.tasks.activeTaskId;
  const sessionId = previous.tasks.activeSessionId;
  const scopeIdentity = stateReadScopeIdentity(current);
  const target = current.tasks.activeSessionId;
  if (
    !taskId ||
    !sessionId ||
    (target !== null && getProvenRemovalPredecessor(store, taskId, target) !== sessionId) ||
    current.tasks.activeTaskId !== taskId ||
    scopeIdentity !== stateReadScopeIdentity(previous)
  )
    return;
  const panel = api.getPanel(`session:${sessionId}`);
  if (!panel || panel.group.activePanel?.id !== panel.id) return;
  const globalPanel = api.activePanel?.id;
  return {
    store,
    taskId,
    previousSessionId: sessionId,
    groupId: panel.group.id,
    navigationRevision: current.taskRemoval.navigationRevision,
    workflowFocus: current.workflowSessionFocus,
    scopeIdentity,
    restorePanelId: globalPanel && !globalPanel.startsWith("session:") ? globalPanel : null,
    cancelled: false,
    focusDelivered: false,
  };
}

function ownsSurface(state: AppState, owner: SurfaceOwner): boolean {
  return (
    state.tasks.activeTaskId === owner.taskId &&
    state.taskRemoval.navigationRevision === owner.navigationRevision &&
    state.workflowSessionFocus === owner.workflowFocus &&
    stateReadScopeIdentity(state) === owner.scopeIdentity
  );
}

function ownsRemovalContext(state: AppState, owner: SurfaceOwner): boolean {
  return (
    state.tasks.activeTaskId === owner.taskId &&
    stateReadScopeIdentity(state) === owner.scopeIdentity
  );
}

function pruneSurfaces(api: DockviewApi, state: AppState): void {
  const owners = surfaces.get(api);
  if (!owners) return;
  for (const [groupId, owner] of owners) {
    const group = api.groups.find((candidate) => candidate.id === groupId);
    if (
      !group ||
      !ownsRemovalContext(state, owner) ||
      (state.tasks.activeSessionId !== null &&
        group.activePanel?.id === `session:${state.tasks.activeSessionId}`)
    )
      owners.delete(groupId);
  }
}

/** Only explicit/proven ownership can expose a removed group's fallback transcript. */
export function setupRemovalSurfaceFocus(api: DockviewApi, store: StoreApi<AppState>): () => void {
  const unsubscribe = store.subscribe((current, previous) => {
    if (useDockviewStore.getState().isRestoringLayout) return;
    pruneSurfaces(api, current);
    const captured = captureVisiblePredecessor(api, store, current, previous);
    if (captured) {
      let owners = surfaces.get(api);
      if (!owners) {
        owners = new Map();
        surfaces.set(api, owners);
      }
      owners.set(captured.groupId, captured);
    }
  });
  const cancel = () => {
    for (const owner of surfaces.get(api)?.values() ?? []) owner.cancelled = true;
  };
  const unsubscribeLayout = useDockviewStore.subscribe((current, previous) => {
    if (current.api !== api || current.currentLayoutEnvId !== previous.currentLayoutEnvId)
      surfaces.delete(api);
    else if (!current.isRestoringLayout) pruneSurfaces(api, store.getState());
  });
  window.addEventListener("pointerdown", cancel, true);
  window.addEventListener("keydown", cancel, true);
  return () => {
    unsubscribe();
    unsubscribeLayout();
    window.removeEventListener("pointerdown", cancel, true);
    window.removeEventListener("keydown", cancel, true);
    surfaces.delete(api);
  };
}

export function getRemovalSurfaceFocus(
  api: DockviewApi,
  store: StoreApi<AppState>,
  taskId: string,
  targetSessionId: string,
): Readonly<Pick<SurfaceOwner, "cancelled" | "restorePanelId" | "groupId">> | undefined {
  const owners = surfaces.get(api);
  const previous = getProvenRemovalPredecessor(store, taskId, targetSessionId);
  if (!owners || !previous) return;
  for (const owner of owners.values()) {
    if (
      owner.store !== store ||
      owner.taskId !== taskId ||
      owner.focusDelivered ||
      owner.previousSessionId !== previous ||
      !ownsSurface(store.getState(), owner)
    )
      continue;
    const group = api.groups.find((candidate) => candidate.id === owner.groupId);
    const independentPanel = group?.activePanel;
    const globalPanel = api.activePanel;
    return {
      groupId: owner.groupId,
      cancelled:
        owner.cancelled && !!independentPanel && !independentPanel.id.startsWith("session:"),
      restorePanelId:
        owner.cancelled && globalPanel && globalPanel.group.id !== owner.groupId
          ? globalPanel.id
          : owner.restorePanelId,
    };
  }
}

export function isRemovalChatBlocked(
  api: DockviewApi | null,
  store: StoreApi<AppState>,
  panelId: string,
  state: AppState,
): boolean {
  const panel = api?.getPanel(panelId);
  const owner = panel && api ? surfaces.get(api)?.get(panel.group.id) : undefined;
  return (
    !!owner &&
    owner.store === store &&
    ownsRemovalContext(state, owner) &&
    isConfirmedSessionRemoved(state.taskSessions, owner.previousSessionId) &&
    panelId !== `session:${state.tasks.activeSessionId}`
  );
}

export function consumeRemovalSurfaceFocus(api: DockviewApi, groupId: string): void {
  const owners = surfaces.get(api);
  const owner = owners?.get(groupId);
  if (!owner) return;
  owner.focusDelivered = true;
  const panel = api.groups.find((group) => group.id === groupId)?.activePanel;
  if (
    !panel ||
    !panel.id.startsWith("session:") ||
    panel.id === `session:${owner.store.getState().tasks.activeSessionId}`
  )
    owners?.delete(groupId);
}

export function shouldPreserveUnrequestedPanel(
  workflowFocusRequestId: number | null,
  removalFocus: ReturnType<typeof getRemovalSurfaceFocus>,
  sessionPanelExistedBefore: boolean,
  activePanelId: string | null,
): boolean {
  return (
    workflowFocusRequestId === null &&
    (!removalFocus || removalFocus.cancelled) &&
    (Boolean(removalFocus?.cancelled) ||
      shouldPreserveActivePanel(sessionPanelExistedBefore, activePanelId))
  );
}
