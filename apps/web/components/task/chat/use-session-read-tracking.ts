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
 * - The anchor is captured synchronously during render (via the React
 *   "adjust state while rendering" pattern), not inside a useEffect, so
 *   it's correct on the exact same commit isVisible first turns true —
 *   never a commit late. isVisible itself (usePanelActive, backed by
 *   Dockview's async active-tab signal) typically only resolves true one
 *   render *after* this hook's owning panel first mounts, so the message
 *   list's very first paint still shows no divider; the message list
 *   (see message-list-native.tsx's didScrollToDivider) re-applies the
 *   scroll once dividerBeforeItemKey subsequently resolves from this
 *   hook, rather than assuming it's already known on mount.
 * - The capture is additionally gated on the session record already
 *   existing in the store (a reactive selector, not store.getState() —
 *   this one specifically must trigger a re-render once the session
 *   loads). Without this, a host where isVisible is true from the very
 *   first render — mobile, which unlike the dockview path never calls
 *   usePanelActive and so has no equivalent async lag — could capture
 *   before the session's own fetch resolves, permanently locking in
 *   `undefined?.last_read_message_id ?? null` as if that were a real
 *   "no prior cursor" answer instead of "haven't loaded it yet".
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
  // The session id this hook currently considers itself "visible for" — null
  // while hidden. The *show* transition (visibleFor !== sessionId while
  // isVisible) is checked on every render, not just in an effect, so the
  // anchor is captured within the same render pass isVisible turns true.
  const [visibleFor, setVisibleFor] = useState<string | null>(null);

  if (unreadDividerEnabled && isVisible && sessionId && sessionLoaded && visibleFor !== sessionId) {
    const priorCursor =
      store.getState().taskSessions.items[sessionId]?.last_read_message_id ?? null;
    setVisibleFor(sessionId);
    setAnchor({ sessionId, messageId: priorCursor });
  }

  // The *hide* transition is debounced rather than immediate: Dockview
  // briefly disposes and re-registers a panel's underlying api during a
  // layout-driven remount (see PanelPortalManager's class doc and
  // usePanelActive), which can make isVisible read false for a render or
  // two even though the user never actually left. Resetting visibleFor
  // synchronously on that blip would make the very next render re-capture
  // a fresh anchor from the cursor's *already-advanced* value, drawing a
  // phantom "New" divider in front of a message the user was actively
  // viewing the whole time. A short delay lets a same-tick blip recover
  // (isVisible flips back true, cancelling the pending reset below) before
  // committing; a genuine navigate-away plays out on a completely
  // different time scale and still resets normally.
  useEffect(() => {
    if (visibleFor === null) return;
    if (!unreadDividerEnabled) {
      setVisibleFor(null);
      setAnchor(null);
      latestDispatchRef.current = null;
      return;
    }
    if (isVisible) return;
    const timer = setTimeout(() => {
      setVisibleFor(null);
      setAnchor(null);
      latestDispatchRef.current = null;
    }, 300);
    return () => clearTimeout(timer);
  }, [isVisible, unreadDividerEnabled, visibleFor]);

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
    // Gated on visibleFor === sessionId (the capture above having already
    // committed for this exact session) rather than just isVisible: without
    // this, this effect could dispatch and advance the cursor on a render
    // before the capture above ever ran for this session (e.g. while
    // sessionLoaded was still false), and the capture would then read the
    // *already-advanced* cursor once it finally runs — silently swallowing
    // this visit's real divider boundary with no error, just no divider.
    if (
      !unreadDividerEnabled ||
      !sessionId ||
      !isVisible ||
      !latestMessageId ||
      visibleFor !== sessionId
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
    visibleFor,
    latestMessageId,
    store,
    unreadDividerEnabled,
    updateSessionReadCursor,
  ]);

  return unreadDividerEnabled && isVisible && anchor && anchor.sessionId === sessionId
    ? anchor.messageId
    : null;
}
