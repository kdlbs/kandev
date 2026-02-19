"use client";

import * as React from "react";
import { cn } from "./lib/utils";

interface ScrollOnOverflowProps {
  children: React.ReactNode;
  className?: string;
  /** Scroll speed in pixels per second. Default 60. */
  speed?: number;
}

/**
 * Container-agnostic wrapper that scrolls its children horizontally on hover
 * when they overflow. Apply any styling to the outer element via className.
 */
const ScrollOnOverflow = React.memo(
  React.forwardRef<HTMLSpanElement, ScrollOnOverflowProps>(function ScrollOnOverflow(
    { children, className, speed = 60 },
    ref,
  ) {
    const outerRef = React.useRef<HTMLSpanElement>(null);
    const innerRef = React.useRef<HTMLSpanElement>(null);
    const timeoutRef = React.useRef<ReturnType<typeof setTimeout>>(undefined);

    React.useEffect(() => {
      return () => clearTimeout(timeoutRef.current);
    }, []);

    const handleMouseEnter = React.useCallback(() => {
      clearTimeout(timeoutRef.current);
      const outer = outerRef.current;
      const inner = innerRef.current;
      if (!outer || !inner) return;

      const overflow = inner.scrollWidth - outer.clientWidth;
      if (overflow <= 0) return;

      const duration = Math.max(1, overflow / speed);
      inner.style.transition = `transform ${duration}s linear`;
      inner.style.transform = `translateX(-${overflow}px)`;

      timeoutRef.current = setTimeout(
        () => {
          inner.style.transition = `transform ${duration}s linear`;
          inner.style.transform = "translateX(0)";
        },
        duration * 1000 + 1500,
      );
    }, [speed]);

    const handleMouseLeave = React.useCallback(() => {
      clearTimeout(timeoutRef.current);
      const inner = innerRef.current;
      if (!inner) return;
      inner.style.transition = "";
      inner.style.transform = "";
    }, []);

    // Merge forwarded ref with internal ref
    const mergedRef = React.useCallback(
      (node: HTMLSpanElement | null) => {
        (outerRef as React.MutableRefObject<HTMLSpanElement | null>).current = node;
        if (typeof ref === "function") ref(node);
        else if (ref) (ref as React.MutableRefObject<HTMLSpanElement | null>).current = node;
      },
      [ref],
    );

    return (
      <span
        ref={mergedRef}
        className={cn("inline-block min-w-0 overflow-hidden", className)}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        <span ref={innerRef} className="inline-block whitespace-nowrap">
          {children}
        </span>
      </span>
    );
  }),
);
ScrollOnOverflow.displayName = "ScrollOnOverflow";

export { ScrollOnOverflow };
