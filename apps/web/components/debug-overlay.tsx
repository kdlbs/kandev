"use client";

import type { PointerEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";

type DebugOverlayProps = {
  title?: string;
  entries: Record<string, unknown>;
  className?: string;
};

function formatValue(value: unknown): string {
  if (value === null) return "null";
  if (value === undefined) return "undefined";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

export function DebugOverlay({ title = "Debug", entries, className }: DebugOverlayProps) {
  const [position, setPosition] = useState({ x: 0, y: 0 });
  const dragOffsetRef = useRef<{ x: number; y: number } | null>(null);

  const positionRef = useRef(position);
  useEffect(() => {
    positionRef.current = position;
  }, [position]);

  const containerRef = useRef<HTMLDivElement | null>(null);
  const [hasCentered, setHasCentered] = useState(false);

  const setContainerRef = (node: HTMLDivElement | null) => {
    containerRef.current = node;
    if (node && !hasCentered) {
      const rect = node.getBoundingClientRect();
      setPosition({
        x: Math.max(12, Math.round(window.innerWidth / 2 - rect.width / 2)),
        y: 24,
      });
      setHasCentered(true);
    }
  };

  const clampPosition = (x: number, y: number) => {
    if (!containerRef.current) {
      return { x, y };
    }
    const rect = containerRef.current.getBoundingClientRect();
    const maxX = Math.max(12, window.innerWidth - rect.width - 12);
    const maxY = Math.max(12, window.innerHeight - rect.height - 12);
    return {
      x: Math.min(Math.max(12, x), maxX),
      y: Math.min(Math.max(12, y), maxY),
    };
  };

  const displayEntries = useMemo(
    () =>
      Object.entries(entries).map(([key, value]) => ({
        key,
        value: formatValue(value),
      })),
    [entries],
  );

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    dragOffsetRef.current = {
      x: event.clientX - positionRef.current.x,
      y: event.clientY - positionRef.current.y,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const handlePointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (!dragOffsetRef.current) return;
    const nextX = event.clientX - dragOffsetRef.current.x;
    const nextY = event.clientY - dragOffsetRef.current.y;
    setPosition(clampPosition(nextX, nextY));
  };

  const handlePointerUp = (event: PointerEvent<HTMLDivElement>) => {
    dragOffsetRef.current = null;
    event.currentTarget.releasePointerCapture(event.pointerId);
  };

  return (
    <div
      className={cn(
        "fixed z-50 min-w-[220px] max-w-[360px] rounded-lg border border-border bg-background/45 p-3 text-xs shadow-lg",
        className,
      )}
      style={{ left: position.x, top: position.y }}
      ref={setContainerRef}
    >
      <div
        className="flex items-center justify-between gap-2 cursor-move select-none border-b border-border/60 pb-2 mb-2 text-[11px] uppercase tracking-wide text-muted-foreground"
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
      >
        <span>{title}</span>
        <span className="text-[10px] opacity-70">drag</span>
      </div>
      <div className="space-y-1">
        {displayEntries.map((entry) => (
          <div key={entry.key} className="flex justify-between gap-2">
            <span className="text-muted-foreground">{entry.key}</span>
            <span className="text-foreground text-right break-words">{entry.value}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
