"use client";

import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

type InlineCodeProps = {
  children: React.ReactNode;
};

const COPIED_RESET_MS = 1500;

type TooltipAnchor = { left: number; top: number };

// Tooltip is portaled to document.body so overflow-hidden ancestors (e.g. the user-message bubble) can't clip it.
export function InlineCode({ children }: InlineCodeProps) {
  const { copy } = useCopyToClipboard();
  const [anchor, setAnchor] = useState<TooltipAnchor | null>(null);
  const [hovered, setHovered] = useState(false);
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const tooltipId = useId();

  useEffect(
    () => () => {
      if (resetTimer.current) clearTimeout(resetTimer.current);
    },
    [],
  );

  // Viewport snapshot; dismiss on scroll/resize rather than repositioning.
  const visible = hovered || copied;
  useEffect(() => {
    if (!visible) return;
    const dismiss = () => {
      setHovered(false);
      setCopied(false);
    };
    window.addEventListener("scroll", dismiss, true);
    window.addEventListener("resize", dismiss);
    return () => {
      window.removeEventListener("scroll", dismiss, true);
      window.removeEventListener("resize", dismiss);
    };
  }, [visible]);

  const anchorTo = (el: HTMLElement) => {
    const rect = el.getBoundingClientRect();
    setAnchor({ left: rect.left + rect.width / 2, top: rect.top });
  };

  const handleClick = async (event: React.MouseEvent<HTMLElement>) => {
    anchorTo(event.currentTarget);
    await copy(String(children));
    setCopied(true);
    if (resetTimer.current) clearTimeout(resetTimer.current);
    resetTimer.current = setTimeout(() => setCopied(false), COPIED_RESET_MS);
  };

  return (
    <>
      <code
        onClick={handleClick}
        onMouseEnter={(event) => {
          anchorTo(event.currentTarget);
          setHovered(true);
        }}
        onMouseLeave={() => setHovered(false)}
        aria-describedby={visible ? tooltipId : undefined}
        className="cursor-pointer hover:bg-foreground/10 transition-colors"
      >
        {children}
      </code>

      {visible &&
        anchor &&
        typeof document !== "undefined" &&
        createPortal(
          <span
            role="tooltip"
            id={tooltipId}
            style={{ left: anchor.left, top: anchor.top - 4 }}
            className={cn(
              "fixed z-50 -translate-x-1/2 -translate-y-full",
              "rounded border border-border bg-popover px-2 py-1 text-xs text-popover-foreground shadow-md",
              "pointer-events-none select-none whitespace-nowrap",
              "animate-in fade-in-0 duration-150",
            )}
          >
            {copied ? "Copied!" : "Copy to clipboard"}
          </span>,
          document.body,
        )}
    </>
  );
}
