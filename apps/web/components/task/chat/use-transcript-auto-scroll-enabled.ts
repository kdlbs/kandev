"use client";

import { useAppStore } from "@/components/state-provider";
import { getStoredAutoScrollEnabled } from "@/lib/local-storage";
import { isTranscriptAutoScrollEnabled } from "@/lib/state/slices/ui/transcript-auto-scroll-selectors";

/**
 * Reactive read of the per-session auto-scroll preference. Shared by the
 * toggle button and both message list renderers so all three consistently
 * default new sessions to enabled. Non-reactive reads/writes (from scroll
 * listeners, imperative Virtuoso callbacks) go straight through
 * `useAppStore.getState()` instead — see message-list-native.tsx /
 * message-list-virtuoso.tsx.
 *
 * Falls back to the sessionStorage-persisted preference (see
 * setTranscriptAutoScrollEnabled) when the in-memory store hasn't seen this
 * session yet — e.g. right after a hard reload or reopening a task in a new
 * tab within the same browser session.
 */
export function useTranscriptAutoScrollEnabled(sessionId: string | null): boolean {
  return useAppStore((state) => {
    const { enabledBySessionId } = state.transcriptAutoScroll;
    if (sessionId && !(sessionId in enabledBySessionId)) {
      const stored = getStoredAutoScrollEnabled(sessionId);
      if (stored !== null) return stored;
    }
    return isTranscriptAutoScrollEnabled(enabledBySessionId, sessionId);
  });
}
