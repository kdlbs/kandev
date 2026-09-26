"use client";

import { useState } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

/**
 * Disclosure state for the queue panel. A pinned session starts open (the
 * caller renders nothing while the queue is empty, so the flag only becomes
 * visible once entries exist, including entries that arrive asynchronously
 * after mount); an unpinned session starts collapsed. The panel collapses on
 * a full drain and follows the target session's pin on session switch.
 * Collapsing via the header X closes it for the current view; a later mount
 * reopens it while the pin is retained. A pin turning on mid-view (another
 * tab's storage event) opens the queue when there is something to show;
 * unpinning never closes an already-open panel. A pinned queue that was
 * empty reopens when entries arrive (initial async load, session switch, or
 * a new message after a full drain); ordinary count changes on a nonempty
 * queue never reopen an X-collapsed panel. An empty queue starts collapsed
 * even when pinned, so unpinning before delayed entries arrive leaves it
 * collapsed. The pin is desktop-only: on phone viewports it is treated as
 * off (the header hides the toggle there, so a pin stored at desktop width
 * must not auto-open the panel with no way to unpin). State is adjusted
 * during render (React docs: "Adjusting some state when a prop changes") to
 * avoid the cascading-render anti-pattern of doing it inside useEffect.
 */
export function useQueuePanelOpenState(
  sessionId: string | null,
  entryCount: number,
  pinned: boolean,
) {
  const { isMobile } = useResponsiveBreakpoint();
  const effectivePinned = isMobile ? false : pinned;
  const [isOpen, setIsOpen] = useState(effectivePinned && entryCount > 0);
  const [lastSession, setLastSession] = useState(sessionId);
  const [lastEntryCount, setLastEntryCount] = useState(entryCount);
  const [lastPinned, setLastPinned] = useState(effectivePinned);
  if (sessionId !== lastSession) {
    setLastSession(sessionId);
    setIsOpen(effectivePinned && entryCount > 0);
  }
  if (entryCount !== lastEntryCount) {
    const wasEmpty = lastEntryCount === 0;
    setLastEntryCount(entryCount);
    if (entryCount === 0) {
      setIsOpen(false);
    } else if (effectivePinned && wasEmpty) {
      setIsOpen(true);
    }
  }
  if (effectivePinned !== lastPinned) {
    setLastPinned(effectivePinned);
    if (effectivePinned && entryCount > 0) setIsOpen(true);
  }
  return [isOpen, setIsOpen] as const;
}
