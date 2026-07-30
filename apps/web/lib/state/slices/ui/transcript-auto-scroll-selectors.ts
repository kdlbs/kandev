/**
 * Auto-scroll is enabled by default for every session; a session only
 * shows up in the map once the user has explicitly toggled it off (or back
 * on) via the transcript's auto-scroll toggle button.
 */
export function isTranscriptAutoScrollEnabled(
  enabledBySessionId: Record<string, boolean>,
  sessionId: string | null,
): boolean {
  if (!sessionId) return true;
  return enabledBySessionId[sessionId] ?? true;
}
