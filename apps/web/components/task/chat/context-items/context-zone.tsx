"use client";

import type { ReactNode } from "react";
import type { ContextItem } from "@/lib/types/context";
import { ContextItemRenderer } from "./context-item-renderer";

type ContextZoneProps = {
  items: ContextItem[];
  sessionId?: string | null;
  queueSlot?: ReactNode;
  todoSlot?: ReactNode;
};

export function ContextZone({
  items,
  sessionId,
  queueSlot,
  todoSlot,
}: ContextZoneProps) {
  const hasContent = !!queueSlot || !!todoSlot || items.length > 0;
  if (!hasContent) return null;

  return (
    <div className="overflow-y-auto max-h-[40%] border-b border-border/50 shrink-0">
      <div className="px-2 pt-2 pb-1 space-y-1.5">
        {queueSlot}
        {todoSlot}
        {items.length > 0 && (
          <div className="flex items-center gap-1 flex-wrap px-0 py-0.5">
            {items.map((item) => (
              <ContextItemRenderer key={item.id} item={item} sessionId={sessionId} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
