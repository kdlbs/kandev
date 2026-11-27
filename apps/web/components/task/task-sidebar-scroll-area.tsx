"use client";

import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type MutableRefObject,
  type ReactNode,
} from "react";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { cn } from "@/lib/utils";

const SCROLL_END_TOLERANCE_PX = 1;

export function TaskSidebarScrollArea({
  children,
  viewportRef,
  className,
  contentClassName = "space-y-4",
  testId = "task-sidebar-scroll",
}: {
  children: ReactNode;
  viewportRef?: MutableRefObject<HTMLDivElement | null>;
  className?: string;
  contentClassName?: string;
  testId?: string;
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const contentRef = useRef<HTMLDivElement | null>(null);
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
      className={cn("task-sidebar-scroll-root min-h-0 flex-1", className)}
      viewportProps={{
        ref: (element: HTMLDivElement | null) => {
          scrollRef.current = element;
          if (viewportRef) viewportRef.current = element;
        },
        // Radix uses an intrinsic-width table wrapper so generic scroll areas
        // can grow horizontally. Sidebar rows must stay viewport-width instead:
        // otherwise a long title pushes its actions beyond the visible edge and
        // never overflows its own ScrollOnOverflow container.
        className: "task-sidebar-scroll [&>div]:!block [&>div]:!min-w-0 [&>div]:!w-full",
        "data-can-scroll-down": canScrollDown,
        "data-testid": testId,
      }}
    >
      <div ref={contentRef} className={contentClassName}>
        {children}
      </div>
    </ScrollArea>
  );
}
