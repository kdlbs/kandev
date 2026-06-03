"use client";

import { cn } from "@/lib/utils";
import type { SearchHit } from "./message-history";

export type HitRowProps = {
  hit: SearchHit;
  isSelected: boolean;
  rowIndex: number;
  onMouseEnter: (rowIndex: number) => void;
  onClick: (historyIndex: number) => void;
};

export function HitRow({ hit, isSelected, rowIndex, onMouseEnter, onClick }: HitRowProps) {
  const firstLine = hit.content.split("\n", 1)[0];
  return (
    <button
      type="button"
      data-hit-index={rowIndex}
      data-testid="history-search-row"
      onMouseEnter={() => onMouseEnter(rowIndex)}
      onClick={() => onClick(hit.index)}
      className={cn(
        "flex w-full cursor-pointer select-none items-center gap-2 rounded-[6px] mx-1 px-2 py-1.5 text-xs text-left",
        "hover:bg-muted/50",
        isSelected && "bg-muted/50",
      )}
      style={{ width: "calc(100% - 8px)" }}
    >
      <span className="truncate flex-1">{firstLine || "(empty)"}</span>
    </button>
  );
}
