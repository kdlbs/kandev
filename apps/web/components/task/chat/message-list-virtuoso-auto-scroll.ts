import { useCallback, useEffect, useRef, useState } from "react";
import type { VirtuosoHandle, StateSnapshot } from "react-virtuoso";
import { useAppStoreApi } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";
import {
  shouldCatchUpOnAutoScrollEnable,
  hasTranscriptAppendedSinceBaseline,
} from "./transcript-auto-scroll";

/**
 * Owns the atBottom tracking, snapshot capture, and append-identity baseline
 * used to decide whether re-enabling auto-scroll should catch the view up to
 * the bottom — see message-list-native.tsx's `useAutoScroll`/
 * `useCatchUpOnReEnable` for the equivalent native-renderer logic.
 * `followOutput` itself is resolved separately by `useGuardedFollowOutput`
 * (see `useVirtuosoCallbacks`) so it also honors the programmatic-scroll
 * lock; this hook only feeds it the atBottom signal via
 * `handleAtBottomStateChange`.
 */
export function useVirtuosoAutoScrollLifecycle(params: {
  sessionId: string | null;
  enabled: boolean;
  messages: Message[];
  virtuosoRef: React.RefObject<VirtuosoHandle | null>;
  firstItemIndex: number;
  itemCount: number;
}) {
  const { sessionId, enabled, messages, virtuosoRef, firstItemIndex, itemCount } = params;
  const storeApi = useAppStoreApi();

  const isAtBottomRef = useRef(true);
  const handleAtBottomStateChange = useCallback((isAtBottom: boolean) => {
    isAtBottomRef.current = isAtBottom;
  }, []);

  const captureSnapshot = useCallback(() => {
    if (!sessionId) return;
    virtuosoRef.current?.getState((state) => {
      storeApi.getState().setTranscriptVirtuosoState(sessionId, state);
    });
  }, [sessionId, storeApi, virtuosoRef]);

  // Capture a restorable snapshot on unmount, so a remounted Virtuoso
  // instance (e.g. navigating away and back while disabled) can pick up
  // exactly where the user left off.
  useEffect(() => captureSnapshot, [captureSnapshot]);

  const baselineRef = useRef<{
    count: number;
    lastId: string | null;
    lastUpdatedAt: string | undefined;
  } | null>(null);
  const hasInitializedBaselineRef = useRef(false);
  const prevEnabledRef = useRef(enabled);
  useEffect(() => {
    const wasEnabled = prevEnabledRef.current;
    prevEnabledRef.current = enabled;
    const captureBaseline = () => {
      const last = messages[messages.length - 1];
      baselineRef.current = {
        count: messages.length,
        lastId: last?.id ?? null,
        lastUpdatedAt: last?.updated_at,
      };
    };

    // First run: if the panel mounted already disabled, there was no
    // in-process disable transition to capture a baseline from —
    // establish one now from the transcript at mount time.
    if (!hasInitializedBaselineRef.current) {
      hasInitializedBaselineRef.current = true;
      if (!enabled) captureBaseline();
      return;
    }

    if (wasEnabled === enabled) return;
    if (!enabled) {
      // Disabling: snapshot immediately, in addition to the unmount capture
      // above, so a remount that happens without an intervening re-render
      // still restores this exact position. Also capture the append-identity
      // baseline used to detect real progression once re-enabled.
      captureSnapshot();
      captureBaseline();
      return;
    }
    // Re-enabling: catch up to the bottom only if the transcript genuinely
    // appended content while disabled and it isn't already in view.
    const baseline = baselineRef.current;
    baselineRef.current = null;
    if (!baseline) return;
    const lastNow = messages[messages.length - 1];
    const appendedSinceDisable = hasTranscriptAppendedSinceBaseline({
      baselineCount: baseline.count,
      currentCount: messages.length,
      baselineLastId: baseline.lastId,
      currentLastId: lastNow?.id ?? null,
      baselineLastUpdatedAt: baseline.lastUpdatedAt,
      currentLastUpdatedAt: lastNow?.updated_at,
    });
    if (
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled,
        nowEnabled: enabled,
        appendedSinceDisable,
        isAtBottom: isAtBottomRef.current,
      })
    ) {
      virtuosoRef.current?.scrollToIndex({ index: firstItemIndex + itemCount - 1, align: "end" });
    }
  }, [enabled, captureSnapshot, firstItemIndex, itemCount, virtuosoRef, messages]);

  // Restore the saved position on first mount when disabled. Lazy-initialized
  // so it's read once at mount time, not on every render.
  const [restoreStateFrom] = useState<StateSnapshot | undefined>(() => {
    if (enabled || !sessionId) return undefined;
    return storeApi.getState().transcriptAutoScroll.virtuosoStateBySessionId[sessionId];
  });

  return { handleAtBottomStateChange, restoreStateFrom };
}
