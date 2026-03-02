"use client";

import { cn } from "@/lib/utils";

type ResizeHandleProps = {
  planModeEnabled?: boolean;
  isAgentBusy?: boolean;
  isStarting?: boolean;
  onMouseDown: (e: React.MouseEvent) => void;
  onDoubleClick: () => void;
};

function getHandleColor(planModeEnabled?: boolean, isAgentBusy?: boolean, isStarting?: boolean) {
  if (isStarting) return "bg-amber-400/60 hover:bg-amber-400";
  if (isAgentBusy && planModeEnabled) return "bg-slate-400/60 hover:bg-slate-400";
  if (isAgentBusy) return "bg-primary/40 hover:bg-primary/70";
  if (planModeEnabled) return "bg-slate-400/60 hover:bg-slate-400";
  return "bg-border hover:bg-muted-foreground";
}

export function ResizeHandle({
  planModeEnabled,
  isAgentBusy,
  isStarting,
  onMouseDown,
  onDoubleClick,
}: ResizeHandleProps) {
  return (
    <button
      type="button"
      className={cn(
        "absolute left-1/2 top-[-1px] -translate-x-1/2 -translate-y-1/2 z-10",
        "w-12 h-2 cursor-ns-resize",
        "flex items-center justify-center",
      )}
      onMouseDown={onMouseDown}
      onDoubleClick={onDoubleClick}
      tabIndex={-1}
    >
      <div
        className={cn(
          "w-8 h-0.5 rounded-full transition-colors",
          getHandleColor(planModeEnabled, isAgentBusy, isStarting),
        )}
      />
    </button>
  );
}
