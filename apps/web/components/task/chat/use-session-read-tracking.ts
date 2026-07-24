"use client";

import { useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { markSessionRead } from "@/lib/api/domains/session-api";

type Anchor = { sessionId: string; messageId: string | null };

/**
 * Tracks a session's Slack-style read cursor and derives the frozen
 * unread-divider anchor for the current "visit".
 *
 * - While the session is the visible chat panel, the cursor advances to
 *   latestMessageId immediately (no debounce) whenever it changes — mirrors
 *   Slack's continuous mark-as-read-while-open behavior, so a quick
 *   navigate-away-and-back still sees the correct boundary next time.
 * - The divider itself never moves during a single visit: it's captured
 *   once, from the cursor's value the instant the session becomes visible
 *   (before this visit's advance overwrites it), and held fixed until the
 *   next visibility transition (leaving and coming back, or switching
 *   sessions). This matches Slack — opening a channel draws the line where
 *   you left off; it doesn't jump around while you're already reading it.
 *
 * Returns the render-item key (see hooks/use-processed-messages.ts's
 * findUnreadDividerItemId) the "New" divider should render immediately
 * before, or null when there's nothing to mark.
 */
export function useSessionReadTracking(
  sessionId: string | null,
  isVisible: boolean,
  latestMessageId: string | null,
): string | null {
  const store = useAppStoreApi();
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const [anchor, setAnchor] = useState<Anchor | null>(null);
  // Tracks the session id we most recently captured an anchor for while
  // visible; reset to null on hide so the next time this (or another)
  // session becomes visible, a fresh anchor is captured.
  const visibleSessionRef = useRef<string | null>(null);
  // Tracks the (sessionId, messageId) of the most recently *dispatched*
  // mark-read request. The backend's cursor write is atomically monotonic
  // (see UpdateTaskSessionLastReadMessageID), but that only protects the
  // persisted row — not an in-flight HTTP response, which is a snapshot
  // frozen at the moment its own GetTaskSession ran. Two overlapping
  // requests (an older m2, then a newer m3) can still have their responses
  // arrive in either order; if m2's (now-stale) response resolves after
  // m3's, applying it verbatim would regress the local store back to m2
  // even though the database is correctly at m3. Comparing against this
  // ref when a response resolves discards any response that's no longer
  // the latest dispatched request for this session.
  const latestDispatchRef = useRef<{ sessionId: string; messageId: string } | null>(null);

  useEffect(() => {
    if (!sessionId || !isVisible) {
      visibleSessionRef.current = null;
      return;
    }
    if (visibleSessionRef.current !== sessionId) {
      const priorCursor =
        store.getState().taskSessions.items[sessionId]?.last_read_message_id ?? null;
      setAnchor({ sessionId, messageId: priorCursor });
      visibleSessionRef.current = sessionId;
    }

    if (!latestMessageId) return;
    const currentCursor = store.getState().taskSessions.items[sessionId]?.last_read_message_id;
    if (currentCursor === latestMessageId) return;
    latestDispatchRef.current = { sessionId, messageId: latestMessageId };
    void markSessionRead(sessionId, latestMessageId)
      .then((response) => {
        const dispatch = latestDispatchRef.current;
        const isStale =
          !dispatch || dispatch.sessionId !== sessionId || dispatch.messageId !== latestMessageId;
        if (isStale) return;
        setTaskSession(response.session);
      })
      .catch((err: unknown) => {
        console.error("Failed to mark session read", err);
      });
  }, [sessionId, isVisible, latestMessageId, store, setTaskSession]);

  return isVisible && anchor && anchor.sessionId === sessionId ? anchor.messageId : null;
}
