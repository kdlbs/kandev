"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ListRange, VirtuosoHandle } from "react-virtuoso";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { useUserMessageNavigation } from "@/hooks/use-message-navigation";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import {
  type ProgrammaticNavigation,
  findNearestUserMessageId,
  getItemKey,
  getNavigationScrollBehavior,
  getUserMessageRenderStops,
  replayMessageHighlight,
  waitForUserMessageElement,
} from "./message-list-shared";

const FIRST_INDEX_BASE = 100_000;
const debugFirstIndex = createDebugLogger("chat:virtuoso:firstIndex");
const debugNavigation = createDebugLogger("chat:virtuoso:navigation");

function computeFirstItemIndex(prevKeys: string[], prevIndex: number, keys: string[]): number {
  if (prevKeys.length > 0 && keys.length > prevKeys.length) {
    const oldFirstKey = prevKeys[0];
    const newPos = keys.indexOf(oldFirstKey);
    if (newPos > 0) return prevIndex - newPos;
    if (newPos === -1) {
      for (let i = 0; i < prevKeys.length; i++) {
        const idx = keys.indexOf(prevKeys[i]);
        if (idx >= 0) return prevIndex - (idx - i);
      }
    }
    return prevIndex;
  }
  if (prevKeys.length === 0 && keys.length > 0) return FIRST_INDEX_BASE - keys.length + 1;
  return prevIndex;
}

type IndexState = { keys: string[]; firstItemIndex: number };

function useStableFirstItemIndex(items: RenderItem[]) {
  const keys = useMemo(() => items.map(getItemKey), [items]);
  const [state, setState] = useState<IndexState>(() => ({
    keys,
    firstItemIndex: FIRST_INDEX_BASE - keys.length + 1,
  }));
  if (keys === state.keys) return state.firstItemIndex;
  const nextIndex = computeFirstItemIndex(state.keys, state.firstItemIndex, keys);
  if (isDebug()) {
    debugFirstIndex("transition", {
      prevKeyCount: state.keys.length,
      nextKeyCount: keys.length,
      prevIndex: state.firstItemIndex,
      nextIndex,
    });
  }
  setState({ keys, firstItemIndex: nextIndex });
  return nextIndex;
}

const NAVIGATION_MOUNT_ATTEMPTS = 4;

export function resolveVirtuosoViewportOrigin(
  scrollParent: HTMLDivElement,
  userStops: ReturnType<typeof getUserMessageRenderStops>,
  visibleRange: ListRange | null,
  firstItemIndex: number,
) {
  const mountedOrigin = findNearestUserMessageId(
    scrollParent,
    userStops.map((stop) => stop.messageId),
  );
  if (mountedOrigin || !visibleRange) return mountedOrigin;
  const midpoint = (visibleRange.startIndex + visibleRange.endIndex) / 2;
  let nearest: { messageId: string; distance: number } | null = null;
  for (const stop of userStops) {
    const distance = Math.abs(firstItemIndex + stop.itemIndex - midpoint);
    if (!nearest || distance < nearest.distance) {
      nearest = { messageId: stop.messageId, distance };
    }
  }
  return nearest?.messageId ?? null;
}

function useViewportOrigin(
  scrollParent: HTMLDivElement | null,
  userStops: ReturnType<typeof getUserMessageRenderStops>,
  firstItemIndex: number,
  setViewportOrigin: (messageId: string | null) => void,
  programmaticRef: React.RefObject<ProgrammaticNavigation>,
) {
  const frameRef = useRef(0);
  const visibleRangeRef = useRef<ListRange | null>(null);
  const updateOrigin = useCallback(() => {
    if (!scrollParent) return;
    const nearestId = resolveVirtuosoViewportOrigin(
      scrollParent,
      userStops,
      visibleRangeRef.current,
      firstItemIndex,
    );
    if (!nearestId && userStops.length > 0) return;
    const programmatic = programmaticRef.current;
    if (programmatic && Date.now() < programmatic.expiresAt) {
      if (nearestId !== programmatic.messageId) return;
    } else if (programmatic) {
      programmaticRef.current = null;
    }
    setViewportOrigin(nearestId);
  }, [firstItemIndex, programmaticRef, scrollParent, setViewportOrigin, userStops]);
  const scheduleUpdate = useCallback(() => {
    cancelAnimationFrame(frameRef.current);
    frameRef.current = requestAnimationFrame(updateOrigin);
  }, [updateOrigin]);
  useEffect(() => {
    if (!scrollParent) return;
    scrollParent.addEventListener("scroll", scheduleUpdate, { passive: true });
    scheduleUpdate();
    return () => {
      cancelAnimationFrame(frameRef.current);
      scrollParent.removeEventListener("scroll", scheduleUpdate);
    };
  }, [scheduleUpdate, scrollParent]);
  return useCallback(
    (range: ListRange) => {
      visibleRangeRef.current = range;
      scheduleUpdate();
    },
    [scheduleUpdate],
  );
}

export function useVirtuosoUserNavigation(args: {
  items: RenderItem[];
  sessionId: string | null;
  scrollParent: HTMLDivElement | null;
  hasMore: boolean;
  oldestCursor: string | null;
  loadMore: () => Promise<number>;
}) {
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const firstItemIndex = useStableFirstItemIndex(args.items);
  const userStops = useMemo(() => getUserMessageRenderStops(args.items), [args.items]);
  const programmaticRef = useRef<ProgrammaticNavigation>(null);
  const mountedRef = useRef(true);
  const sessionIdRef = useRef(args.sessionId);
  sessionIdRef.current = args.sessionId;
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);
  const navigateTo = useCallback(
    async (messageId: string) => {
      if (!args.scrollParent || !virtuosoRef.current || !args.sessionId) return false;
      const stop = userStops.find((entry) => entry.messageId === messageId);
      if (!stop) return false;
      const actionSessionId = args.sessionId;
      const previousScrollTop = args.scrollParent.scrollTop;
      programmaticRef.current = { messageId, expiresAt: Date.now() + 3000 };
      if (isDebug()) {
        debugNavigation("scroll.start", {
          messageId,
          firstItemIndex,
          itemIndex: stop.itemIndex,
          virtuosoIndex: stop.itemIndex,
          stopCount: userStops.length,
          previousScrollTop,
        });
      }
      let element: HTMLElement | null = null;
      for (let attempt = 0; attempt < NAVIGATION_MOUNT_ATTEMPTS && !element; attempt++) {
        // The imperative API is zero-based against totalCount; firstItemIndex only offsets rows.
        virtuosoRef.current.scrollToIndex({
          index: stop.itemIndex,
          align: "center",
          behavior: attempt === 0 ? getNavigationScrollBehavior() : "auto",
        });
        element = await waitForUserMessageElement(
          args.scrollParent,
          messageId,
          () => mountedRef.current && sessionIdRef.current === actionSessionId,
        );
      }
      if (!element || sessionIdRef.current !== actionSessionId) {
        if (isDebug()) {
          debugNavigation("scroll.mount-failed", {
            messageId,
            sessionChanged: sessionIdRef.current !== actionSessionId,
            mounted: mountedRef.current,
            currentScrollTop: args.scrollParent.scrollTop,
            restoreScrollTop: previousScrollTop,
            mountedUserMessageIds: Array.from(
              args.scrollParent.querySelectorAll<HTMLElement>("[data-user-message-id]"),
            ).map((node) => node.dataset.userMessageId),
          });
        }
        programmaticRef.current = null;
        if (mountedRef.current && sessionIdRef.current === actionSessionId) {
          args.scrollParent.scrollTop = previousScrollTop;
        }
        return false;
      }
      if (isDebug()) debugNavigation("scroll.complete", { messageId });
      programmaticRef.current = { messageId, expiresAt: Date.now() + 500 };
      replayMessageHighlight(element);
      return true;
    },
    [args.scrollParent, args.sessionId, firstItemIndex, userStops],
  );
  const navigation = useUserMessageNavigation({
    sessionId: args.sessionId,
    items: args.items,
    hasOlder: args.hasMore,
    oldestCursor: args.oldestCursor,
    loadOlder: args.loadMore,
    navigateTo,
  });
  const onVisibleRangeChanged = useViewportOrigin(
    args.scrollParent,
    userStops,
    firstItemIndex,
    navigation.setViewportOrigin,
    programmaticRef,
  );
  return { navigation, onVisibleRangeChanged, firstItemIndex, virtuosoRef };
}
