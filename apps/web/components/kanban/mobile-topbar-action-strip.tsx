"use client";

import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";

const SCROLL_END_TOLERANCE_PX = 1;

type ScrollCues = {
  left: boolean;
  right: boolean;
};

function scrollCuesFor(element: HTMLDivElement): ScrollCues {
  const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
  return {
    left: element.scrollLeft > SCROLL_END_TOLERANCE_PX,
    right: maxScrollLeft - element.scrollLeft > SCROLL_END_TOLERANCE_PX,
  };
}

export function MobileTopbarActionStrip({ children }: { children: ReactNode }) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [scrollCues, setScrollCues] = useState<ScrollCues>({ left: false, right: false });

  const updateScrollCues = useCallback(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const next = scrollCuesFor(viewport);
    setScrollCues((current) =>
      current.left === next.left && current.right === next.right ? current : next,
    );
  }, []);

  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;

    updateScrollCues();
    viewport.addEventListener("scroll", updateScrollCues, { passive: true });
    window.addEventListener("resize", updateScrollCues);

    const observer =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(updateScrollCues);
    observer?.observe(viewport);
    if (contentRef.current) observer?.observe(contentRef.current);

    return () => {
      viewport.removeEventListener("scroll", updateScrollCues);
      window.removeEventListener("resize", updateScrollCues);
      observer?.disconnect();
    };
  }, [updateScrollCues]);

  return (
    <div
      className="relative h-8 min-w-0 flex-1"
      data-testid="mobile-topbar-action-strip"
      data-can-scroll-left={scrollCues.left}
      data-can-scroll-right={scrollCues.right}
    >
      <div
        ref={viewportRef}
        className="absolute inset-x-0 top-1/2 h-11 min-w-0 -translate-y-1/2 overflow-x-auto overscroll-x-contain scrollbar-hide"
        data-testid="mobile-topbar-action-strip-viewport"
      >
        <div
          ref={contentRef}
          className="flex h-full w-max min-w-full items-center justify-end gap-2 pr-1"
        >
          {children}
        </div>
      </div>
      {scrollCues.left && (
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 left-0 z-10 w-7 bg-gradient-to-r from-background via-background/80 to-transparent"
          data-testid="mobile-topbar-action-strip-left-fade"
        />
      )}
      {scrollCues.right && (
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 right-0 z-10 w-7 bg-gradient-to-l from-background via-background/80 to-transparent"
          data-testid="mobile-topbar-action-strip-right-fade"
        />
      )}
    </div>
  );
}
