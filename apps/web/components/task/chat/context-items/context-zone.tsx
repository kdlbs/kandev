"use client";

import type { ContextItem } from "@/lib/types/context";
import { ContextItemRenderer } from "./context-item-renderer";

type ContextZoneProps = {
  items: ContextItem[];
  sessionId?: string | null;
};

export function ContextZone({ items, sessionId }: ContextZoneProps) {
  if (items.length === 0) return null;

  return (
    <div className="max-h-28 min-w-0 shrink-0 overflow-y-auto border-b border-border/50">
      <div className="min-w-0 space-y-1.5 px-2 pt-2 pb-1">
        <div className="flex min-w-0 flex-wrap items-center gap-1 px-0 py-0.5">
          {items.map((item) => (
            <ContextItemRenderer key={item.id} item={item} sessionId={sessionId} />
          ))}
        </div>
      </div>
    </div>
  );
}
