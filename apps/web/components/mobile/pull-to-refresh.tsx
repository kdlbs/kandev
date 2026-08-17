"use client";

import { useRef, useState, type ReactNode, type TouchEvent } from "react";
import { IconRefresh } from "@tabler/icons-react";
import {
  PULL_TO_REFRESH_THRESHOLD,
  isAtVerticalScrollTop,
  pullDistance,
  shouldRefreshAfterPull,
} from "@/lib/mobile/pull-to-refresh";
import { t } from "@/lib/i18n";

type PullState = { x: number; y: number; eligible: boolean } | null;

function refreshLabel(refreshing: boolean, distance: number): string {
  if (refreshing) return t("kanban:pullToRefreshRefreshing");
  if (distance >= PULL_TO_REFRESH_THRESHOLD) return t("kanban:pullToRefreshRelease");
  return t("kanban:pullToRefreshPull");
}

export function PullToRefresh({
  children,
  onRefresh,
}: {
  children: ReactNode;
  onRefresh: () => void | Promise<void>;
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  const pullRef = useRef<PullState>(null);
  const distanceRef = useRef(0);
  const [distance, setDistance] = useState(0);
  const [refreshing, setRefreshing] = useState(false);

  const onTouchStart = (event: TouchEvent<HTMLDivElement>) => {
    if (refreshing || event.touches.length !== 1) return;
    const touch = event.touches[0];
    pullRef.current = {
      x: touch.clientX,
      y: touch.clientY,
      eligible: isAtVerticalScrollTop(event.target, rootRef.current),
    };
  };

  const onTouchMove = (event: TouchEvent<HTMLDivElement>) => {
    const pull = pullRef.current;
    if (!pull || !pull.eligible) return;
    if (event.touches.length !== 1) {
      pullRef.current = null;
      distanceRef.current = 0;
      setDistance(0);
      return;
    }
    const touch = event.touches[0];
    const dx = touch.clientX - pull.x;
    const dy = touch.clientY - pull.y;
    if (Math.abs(dx) > Math.abs(dy) || dy <= 0) {
      pullRef.current = null;
      distanceRef.current = 0;
      setDistance(0);
      return;
    }
    event.preventDefault();
    const nextDistance = pullDistance(dy);
    distanceRef.current = nextDistance;
    setDistance(nextDistance);
  };

  const onTouchEnd = () => {
    const shouldRefresh = shouldRefreshAfterPull(distanceRef.current);
    pullRef.current = null;
    distanceRef.current = 0;
    setDistance(0);
    if (!shouldRefresh || refreshing) return;
    setRefreshing(true);
    Promise.resolve(onRefresh()).finally(() => setRefreshing(false));
  };

  const visible = refreshing || distance > 0;
  return (
    <div
      ref={rootRef}
      className="relative flex min-h-0 flex-1 flex-col"
      onTouchStartCapture={onTouchStart}
      onTouchMoveCapture={onTouchMove}
      onTouchEndCapture={onTouchEnd}
      onTouchCancelCapture={() => {
        pullRef.current = null;
        distanceRef.current = 0;
        setDistance(0);
      }}
      data-testid="pull-to-refresh"
    >
      {visible && (
        <div
          className="pointer-events-none absolute inset-x-0 top-1 z-20 flex justify-center text-muted-foreground"
          data-testid="pull-to-refresh-indicator"
          aria-live="polite"
        >
          <IconRefresh className={refreshing ? "size-5 animate-spin" : "size-5"} />
          <span className="sr-only">{refreshLabel(refreshing, distance)}</span>
        </div>
      )}
      {children}
    </div>
  );
}
