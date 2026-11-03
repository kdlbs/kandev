"use client";

import { useAppStore } from "@/components/state-provider";

/** Unread-dot preferences plus the displayed session the open task excludes. */
export function useUnreadTaskDotDisplay() {
  const unreadMessagesEnabled = useAppStore((state) => state.userSettings.unreadTaskDot);
  const delaySeconds = useAppStore((state) => state.userSettings.unreadTaskDotHideDelaySeconds);
  const displayedSessionId = useAppStore((state) => state.tasks.activeSessionId);
  return {
    unreadMessagesEnabled,
    unreadDotHideDelayMs: delaySeconds * 1000,
    displayedSessionId,
  };
}
