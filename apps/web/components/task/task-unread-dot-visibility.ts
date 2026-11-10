/**
 * The single display rule for a task row's unread dot. The setting hides
 * every dot. A task that is not open shows the dot while any session is
 * unread. The open task shows it while lingering after selection, or while a
 * session other than the displayed one is unread, because the displayed
 * session is marked read as the user views it.
 */
export function shouldShowUnreadDot(input: {
  enabled: boolean;
  unreadSessionIds: string[] | undefined;
  isSelected: boolean;
  lingering: boolean;
  displayedSessionId: string | null;
}): boolean {
  if (!input.enabled) return false;
  if (input.lingering) return true;
  const ids = input.unreadSessionIds ?? [];
  if (!input.isSelected) return ids.length > 0;
  return ids.some((id) => id !== input.displayedSessionId);
}
