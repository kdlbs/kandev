import type { DockviewApi } from "dockview-react";

export type SessionPanelActivationArgs = {
  sessionPanelExistedBefore: boolean;
  prevTaskId: string | null;
  prevSessionId: string | null;
  currentTaskId: string | null;
  currentSessionId: string;
  currentActivePanelId: string | null;
};

export function shouldPreserveActivePanel(
  sessionPanelExistedBefore: boolean,
  activePanelId: string | null,
): boolean {
  return !sessionPanelExistedBefore && !!activePanelId && activePanelId !== "chat";
}

export function isChatPlaceholderSelected(api: DockviewApi): boolean {
  const chatPanel = api.getPanel("chat");
  return chatPanel?.group.activePanel?.id === chatPanel?.id;
}

export function activateChatReplacement(
  api: DockviewApi,
  effectiveSessionId: string,
  shouldActivateReplacement: boolean,
): void {
  if (!shouldActivateReplacement) return;
  api.getPanel(`session:${effectiveSessionId}`)?.api.setActive();
}

export function restorePreservedActivePanel(
  api: DockviewApi,
  panelId: string | null,
  shouldRestore: boolean,
): void {
  if (!shouldRestore || !panelId) return;
  api.getPanel(panelId)?.api.setActive();
}

export function shouldActivateSessionPanel(args: SessionPanelActivationArgs): boolean {
  const {
    sessionPanelExistedBefore,
    prevTaskId,
    prevSessionId,
    currentTaskId,
    currentSessionId,
    currentActivePanelId,
  } = args;
  const sessionPanelId = `session:${currentSessionId}`;
  if (!sessionPanelExistedBefore) {
    return (
      !currentActivePanelId ||
      currentActivePanelId === "chat" ||
      currentActivePanelId === sessionPanelId
    );
  }
  const isFirstMount = prevTaskId === null && prevSessionId === null;
  if (isFirstMount) {
    return !currentActivePanelId || currentActivePanelId === sessionPanelId;
  }
  const taskChanged = prevTaskId !== currentTaskId;
  const sessionChanged = prevSessionId !== null && prevSessionId !== currentSessionId;
  return sessionChanged && !taskChanged;
}
