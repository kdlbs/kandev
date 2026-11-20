import type { DockviewApi } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { getProvenRemovalPredecessor } from "@/lib/state/session-removal-handoff";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { shouldPreserveActivePanel } from "./dockview-session-tab-activation";

type SurfaceOwner = {
  store: StoreApi<AppState>;
  taskId: string;
  previousSessionId: string;
  navigationRevision: number;
  workflowFocus: AppState["workflowSessionFocus"];
  scopeIdentity: string;
  restorePanelId: string | null;
  cancelled: boolean;
};
const surfaces = new WeakMap<DockviewApi, SurfaceOwner>();

function sameContext(current: AppState, previous: AppState): boolean {
  return (
    current.tasks.activeTaskId === previous.tasks.activeTaskId &&
    current.taskRemoval.navigationRevision === previous.taskRemoval.navigationRevision &&
    current.workflowSessionFocus === previous.workflowSessionFocus &&
    stateReadScopeIdentity(current) === stateReadScopeIdentity(previous)
  );
}

function captureVisiblePredecessor(
  api: DockviewApi,
  store: StoreApi<AppState>,
  current: AppState,
  previous: AppState,
): SurfaceOwner | undefined {
  const taskId = previous.tasks.activeTaskId;
  const sessionId = previous.tasks.activeSessionId;
  if (
    !taskId ||
    !sessionId ||
    current.tasks.activeSessionId !== null ||
    !sameContext(current, previous)
  )
    return;
  const focus = current.workflowSessionFocus;
  if (focus.intent?.taskId === taskId || focus.request?.taskId === taskId) return;
  const panel = api.getPanel(`session:${sessionId}`);
  if (!panel || panel.group.activePanel?.id !== panel.id) return;
  const globalPanel = api.activePanel?.id;
  return {
    store,
    taskId,
    previousSessionId: sessionId,
    navigationRevision: current.taskRemoval.navigationRevision,
    workflowFocus: focus,
    scopeIdentity: stateReadScopeIdentity(current),
    restorePanelId: globalPanel && !globalPanel.startsWith("session:") ? globalPanel : null,
    cancelled: false,
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

/** A blank interval retains only the conversation that was actually visible. */
export function setupRemovalSurfaceFocus(api: DockviewApi, store: StoreApi<AppState>): () => void {
  const unsubscribe = store.subscribe((current, previous) => {
    const owner = surfaces.get(api);
    if (
      owner &&
      (!ownsSurface(current, owner) ||
        (current.tasks.activeSessionId !== null &&
          getProvenRemovalPredecessor(store, owner.taskId, current.tasks.activeSessionId) !==
            owner.previousSessionId))
    )
      surfaces.delete(api);
    if (!useDockviewStore.getState().isRestoringLayout) {
      const captured = captureVisiblePredecessor(api, store, current, previous);
      if (captured) surfaces.set(api, captured);
    }
  });
  const cancel = () => {
    const owner = surfaces.get(api);
    if (owner) owner.cancelled = true;
  };
  const unsubscribeLayout = useDockviewStore.subscribe((current) => {
    if (current.isRestoringLayout || current.api !== api) surfaces.delete(api);
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
): Readonly<Pick<SurfaceOwner, "cancelled" | "restorePanelId">> | undefined {
  const owner = surfaces.get(api);
  if (
    !owner ||
    owner.store !== store ||
    owner.taskId !== taskId ||
    !ownsSurface(store.getState(), owner)
  )
    return;
  if (getProvenRemovalPredecessor(store, taskId, targetSessionId) !== owner.previousSessionId)
    return;
  return owner;
}

export function consumeRemovalSurfaceFocus(api: DockviewApi): void {
  surfaces.delete(api);
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
