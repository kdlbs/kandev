"use client";

import type { ReactNode } from "react";
import type { ContextItem } from "@/lib/types/context";
import { cn } from "@/lib/utils";
import { ContextItemRenderer } from "./context-item-renderer";

type ContextZoneProps = {
  items: ContextItem[];
  sessionId?: string | null;
  leadingContent?: ReactNode;
  rowClassName?: string;
  scrollable?: boolean;
};

export function ContextZone({
  items,
  sessionId,
  leadingContent,
  rowClassName,
  scrollable = true,
}: ContextZoneProps) {
  if (items.length === 0 && !leadingContent) return null;

  return (
    <div
      className={cn(
        "min-w-0 shrink-0 border-b border-border/50",
        scrollable && "max-h-28 overflow-y-auto",
      )}
    >
      <div
        className={cn("flex min-w-0 flex-wrap items-center gap-1 px-2 pt-2.5 pb-1.5", rowClassName)}
        data-testid="composer-context-row"
      >
        {leadingContent}
        {items.map((item) => (
          <ContextItemRenderer key={item.id} item={item} sessionId={sessionId} />
        ))}
      </div>
    </div>
  );
}
