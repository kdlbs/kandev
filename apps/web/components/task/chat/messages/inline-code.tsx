"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

type InlineCodeProps = {
  children: React.ReactNode;
};

const COPIED_RESET_MS = 1500;

type TooltipAnchor = { left: number; top: number };

/**
 * Inline `code` span with a click-to-copy affordance.
 *
 * The hover / "Copied!" tooltip is rendered through a portal on `document.body`
 * rather than as an absolutely-positioned child. Inline code frequently lives
 * inside containers that clip their overflow (e.g. the rounded user-message
 * bubble, which nests several `overflow-hidden` layers); an in-flow absolute
 * tooltip gets clipped by those ancestors. Portaling it to the body lets it
 * float freely so it is never clipped.
 */
export function InlineCode({ children }: InlineCodeProps) {
  const { copy } = useCopyToClipboard();
  const [anchor, setAnchor] = useState<TooltipAnchor | null>(null);
  const [hovered, setHovered] = useState(false);
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current) clearTimeout(resetTimer.current);
    },
    [],
  );

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
        className="cursor-pointer hover:bg-foreground/10 transition-colors"
      >
        {children}
      </code>

      {(hovered || copied) &&
        anchor &&
        typeof document !== "undefined" &&
        createPortal(
          <span
            role="tooltip"
            style={{ left: anchor.left, top: anchor.top - 4 }}
            className={cn(
              "fixed z-50 -translate-x-1/2 -translate-y-full",
              "rounded border border-border bg-popover px-2 py-1 text-xs text-popover-foreground shadow-md",
              "pointer-events-none select-none whitespace-nowrap",
            )}
          >
            {copied ? "Copied!" : "Copy to clipboard"}
          </span>,
          document.body,
        )}
    </>
  );
}
