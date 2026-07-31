"use client";

import { useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { markSessionRead } from "@/lib/api/domains/session-api";

type Anchor = { sessionId: string; messageId: string | null };
type Visit = {
  sessionId: string;
  priorCursor: string | null;
  initialMessagesReady: boolean;
};

function visitAnchor(
  sessionId: string,
  priorCursor: string | null,
  latestMessageId: string | null,
) {
  return latestMessageId !== null && priorCursor !== null && priorCursor !== latestMessageId
    ? { sessionId, messageId: priorCursor }
    : null;
}

function visibleAnchor(
  unreadDividerEnabled: boolean,
  isVisible: boolean,
  anchor: Anchor | null,
  sessionId: string | null,
): string | null {
  if (!unreadDividerEnabled || !isVisible || anchor?.sessionId !== sessionId) return null;
  return anchor.messageId;
}

/**
 * Tracks a session's Slack-style read cursor and derives the frozen
 * unread-divider anchor for the current "visit".
 *
 * - While the session is the visible chat panel, the cursor advances to
 *   latestMessageId immediately (no debounce) whenever it changes — mirrors
 *   Slack's continuous mark-as-read-while-open behavior, so a quick
 *   navigate-away-and-back still sees the correct boundary next time.
 * - A visible visit captures its persisted cursor synchronously, but decides
 *   divider eligibility only once the initial message load settles. If that
 *   cursor is already the initial transcript tail, the visit is permanently
 *   divider-ineligible: later live messages cannot create a "New" boundary.
 *   A genuine initial unread boundary remains frozen until the next
 *   visibility transition (leaving and coming back, or switching sessions).
 * - The visit state is captured during render (via the React "adjust state
 *   while rendering" pattern), not inside a useEffect. It retains the cursor
 *   from before live mark-read can overwrite it, while avoiding a divider
 *   decision from a partial transcript.
 * - The capture is additionally gated on the session record already
 *   existing in the store (a reactive selector, not store.getState()). This
 *   prevents a host where isVisible is true from its first render (such as
 *   mobile) from treating an unloaded session as a real empty cursor.
 *
 * Returns the render-item key (see hooks/use-processed-messages.ts's
 * findUnreadDividerItemId) the "New" divider should render immediately
 * before, or null when there's nothing to mark.
 */
export function useSessionReadTracking(
  sessionId: string | null,
  isVisible: boolean,
  latestMessageId: string | null,
  initialMessagesLoading = false,
): string | null {
  const store = useAppStoreApi();
  const updateSessionReadCursor = useAppStore((state) => state.updateSessionReadCursor);
  const unreadDividerEnabled = useAppStore((state) => state.userSettings.unreadDivider);
  // Reactive (not store.getState()) so a session that hasn't loaded into the
  // store yet at mount — e.g. mobile, where isVisible defaults to true from
  // the very first render (see TaskChatPanelProps), unlike the dockview host
  // path where usePanelActive's async lag incidentally gives the session
  // fetch a head start — triggers a re-render once it does, instead of the
  // capture below racing ahead of it and locking in undefined?.last_read_
  // message_id ?? null as if that were a real "no prior cursor" answer.
  const sessionLoaded = useAppStore(
    (state) => sessionId !== null && sessionId in state.taskSessions.items,
  );
  const [anchor, setAnchor] = useState<Anchor | null>(null);
  // A visit starts at a visibility transition but only becomes eligible for
  // a divider after its initial message load settles. Its prior cursor stays
  // immutable so live messages cannot create a new boundary mid-visit.
  const [visit, setVisit] = useState<Visit | null>(null);

  if (unreadDividerEnabled && isVisible && sessionId && sessionLoaded) {
    if (visit?.sessionId !== sessionId) {
      const priorCursor =
        store.getState().taskSessions.items[sessionId]?.last_read_message_id ?? null;
      const initialMessagesReady = !initialMessagesLoading;
      setVisit({ sessionId, priorCursor, initialMessagesReady });
      setAnchor(initialMessagesReady ? visitAnchor(sessionId, priorCursor, latestMessageId) : null);
    } else if (!visit.initialMessagesReady && !initialMessagesLoading) {
      setVisit({ ...visit, initialMessagesReady: true });
      setAnchor(visitAnchor(sessionId, visit.priorCursor, latestMessageId));
    }
  }

  // The *hide* transition is debounced rather than immediate: Dockview
  // briefly disposes and re-registers a panel's underlying api during a
  // layout-driven remount (see PanelPortalManager's class doc and
  // usePanelActive), which can make isVisible read false for a render or
  // two even though the user never actually left.
  useEffect(() => {
    if (visit === null) return;
    if (!unreadDividerEnabled) {
      setVisit(null);
      setAnchor(null);
      latestDispatchRef.current = null;
      return;
    }
    if (isVisible) return;
    const timer = setTimeout(() => {
      setVisit(null);
      setAnchor(null);
      latestDispatchRef.current = null;
    }, 300);
    return () => clearTimeout(timer);
  }, [isVisible, unreadDividerEnabled, visit]);

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
    if (
      !unreadDividerEnabled ||
      !sessionId ||
      !isVisible ||
      !latestMessageId ||
      visit?.sessionId !== sessionId ||
      !visit.initialMessagesReady
    )
      return;
    const currentCursor = store.getState().taskSessions.items[sessionId]?.last_read_message_id;
    if (currentCursor === latestMessageId) return;
    latestDispatchRef.current = { sessionId, messageId: latestMessageId };
    void markSessionRead(sessionId, latestMessageId)
      .then((response) => {
        const dispatch = latestDispatchRef.current;
        const isStale =
          !dispatch || dispatch.sessionId !== sessionId || dispatch.messageId !== latestMessageId;
        if (isStale) return;
        updateSessionReadCursor(response.session_id, response.last_read_message_id);
      })
      .catch((err: unknown) => {
        console.error("Failed to mark session read", err);
      });
  }, [
    sessionId,
    isVisible,
    visit,
    latestMessageId,
    store,
    unreadDividerEnabled,
    updateSessionReadCursor,
  ]);

  return visibleAnchor(unreadDividerEnabled, isVisible, anchor, sessionId);
}
