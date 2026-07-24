"use client";

import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ScrollArea } from "@kandev/ui/scroll-area";

const SCROLL_END_TOLERANCE_PX = 1;

export function TaskSidebarScrollArea({ children }: { children: ReactNode }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [canScrollDown, setCanScrollDown] = useState(false);

  const updateScrollCue = useCallback(() => {
    const element = scrollRef.current;
    if (!element) return;
    const remaining = element.scrollHeight - element.clientHeight - element.scrollTop;
    setCanScrollDown(remaining > SCROLL_END_TOLERANCE_PX);
  }, []);

  useLayoutEffect(() => {
    const scrollElement = scrollRef.current;
    if (!scrollElement) return;

    updateScrollCue();
    scrollElement.addEventListener("scroll", updateScrollCue, { passive: true });
    window.addEventListener("resize", updateScrollCue);

    const observer =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(updateScrollCue);
    observer?.observe(scrollElement);
    if (contentRef.current) observer?.observe(contentRef.current);

    return () => {
      scrollElement.removeEventListener("scroll", updateScrollCue);
      window.removeEventListener("resize", updateScrollCue);
      observer?.disconnect();
    };
  }, [updateScrollCue]);

  return (
    <ScrollArea
      type="auto"
      className="task-sidebar-scroll-root min-h-0 flex-1"
      viewportProps={{
        ref: scrollRef,
        className: "task-sidebar-scroll",
        "data-can-scroll-down": canScrollDown,
        "data-testid": "task-sidebar-scroll",
      }}
    >
      <div ref={contentRef} className="space-y-4">
        {children}
      </div>
    </ScrollArea>
  );
}
