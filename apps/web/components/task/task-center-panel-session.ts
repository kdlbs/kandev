export function resolveCenterPanelSessionId(
  explicitSessionId: string | null | undefined,
  activeSessionId: string | null,
): string | null {
  return explicitSessionId ?? activeSessionId;
}
