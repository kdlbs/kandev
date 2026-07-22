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
