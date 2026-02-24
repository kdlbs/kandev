"use client";

import { cn } from "@/lib/utils";

type ResizeHandleProps = {
  visible: boolean;
  planModeEnabled?: boolean;
  onMouseDown: (e: React.MouseEvent) => void;
  onDoubleClick: () => void;
};

export function ResizeHandle({
  visible,
  planModeEnabled,
  onMouseDown,
  onDoubleClick,
}: ResizeHandleProps) {
  return (
    <button
      type="button"
      className={cn(
        "absolute left-1/2 top-[-1px] -translate-x-1/2 -translate-y-1/2 z-10",
        "w-12 h-2 cursor-ns-resize transition-opacity",
        "flex items-center justify-center",
        visible ? "opacity-100" : "opacity-0 pointer-events-none",
      )}
      onMouseDown={onMouseDown}
      onDoubleClick={onDoubleClick}
      tabIndex={-1}
    >
      <div
        className={cn(
          "w-8 h-0.5 rounded-full transition-colors",
          planModeEnabled
            ? "bg-slate-400/60 hover:bg-slate-400"
            : "bg-border hover:bg-muted-foreground",
        )}
      />
    </button>
  );
}
