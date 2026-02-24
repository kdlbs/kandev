"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

type InlineCodeProps = {
  children: React.ReactNode;
};

export function InlineCode({ children }: InlineCodeProps) {
  const { copy } = useCopyToClipboard();
  const [showTooltip, setShowTooltip] = useState(false);
  const [tooltipText, setTooltipText] = useState("Copy to clipboard");

  const handleClick = async () => {
    const text = String(children);
    await copy(text);
    setTooltipText("Copied!");
    setShowTooltip(true);

    setTimeout(() => {
      setShowTooltip(false);
      setTimeout(() => setTooltipText("Copy to clipboard"), 200);
    }, 1500);
  };

  return (
    <span className="relative inline-block group/inline-code">
      <code
        onClick={handleClick}
        className={cn(
          "px-1 py-0.5 bg-foreground/[0.06] text-foreground/80 rounded font-mono",
          "cursor-pointer hover:bg-foreground/10 transition-colors",
        )}
      >
        {children}
      </code>

      {/* Tooltip */}
      <span
        className={cn(
          "absolute bottom-full left-1/2 -translate-x-1/2 mb-1",
          "px-2 py-1 text-xs text-white bg-gray-900 rounded whitespace-nowrap",
          "pointer-events-none transition-opacity duration-200",
          "opacity-0 group-hover/inline-code:opacity-100",
          showTooltip && "opacity-100",
        )}
      >
        {tooltipText}
        <span className="absolute top-full left-1/2 -translate-x-1/2 -mt-px border-4 border-transparent border-t-gray-900" />
      </span>
    </span>
  );
}
