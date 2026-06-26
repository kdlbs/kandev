export function resolveCenterPanelSessionId(
  explicitSessionId: string | null | undefined,
  activeSessionId: string | null,
  activeSessionTaskId: string | null | undefined,
  activeTaskId: string | null,
): string | null {
  if (explicitSessionId != null) return explicitSessionId;
  if (!activeSessionId || !activeTaskId || activeSessionTaskId !== activeTaskId) return null;
  return activeSessionId;
}
