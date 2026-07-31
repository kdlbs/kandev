"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchTaskSession } from "@/lib/api/domains/session-api";

/**
 * Guarantees the store holds the session row for `sessionId`.
 *
 * A quick chat learned from a task event carries only the tab's identity — the
 * event has no session payload. Without the row the tab is inert: `useSession`
 * returns null so it never subscribes, and `requireSessionInputMode` rejects
 * every send. Fetching lazily, when the user actually opens that tab, keeps the
 * cross-device tab strip cheap while making any tab openable regardless of
 * which path introduced it.
 */
export function useEnsureTaskSession(sessionId: string | null): void {
  const hasSession = useAppStore((state) =>
    sessionId ? Boolean(state.taskSessions.items[sessionId]) : true,
  );
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  // Never re-request a session id this hook has already asked for; a chat whose
  // task was deleted elsewhere would otherwise refetch on every render.
  const requested = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!sessionId || hasSession || requested.current.has(sessionId)) return;
    requested.current.add(sessionId);

    let cancelled = false;
    fetchTaskSession(sessionId)
      .then((response) => {
        if (!cancelled && response.session) setTaskSession(response.session);
      })
      .catch(() => {
        // The session may be genuinely gone (deleted on another device); the
        // tab strip drops it on the next event or resync.
      });

    return () => {
      cancelled = true;
    };
  }, [sessionId, hasSession, setTaskSession]);
}
