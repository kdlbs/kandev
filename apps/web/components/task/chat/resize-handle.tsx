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
  if (isStarting) return "bg-primary/70 hover:bg-primary";
  if (isAgentBusy && planModeEnabled) return "bg-violet-400/80 hover:bg-violet-400";
  if (isAgentBusy) return "bg-primary/70 hover:bg-primary";
  if (planModeEnabled) return "bg-violet-400/80 hover:bg-violet-400";
  // Resting state: muted-foreground (light gray on dark bg) at high opacity so
  // the handle reads as a discoverable affordance, not invisible chrome.
  return "bg-muted-foreground/60 hover:bg-muted-foreground";
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
      aria-label="Resize"
      className={cn(
        "absolute left-1/2 top-[-1px] -translate-x-1/2 -translate-y-1/2 z-10",
        "w-16 h-3 cursor-ns-resize",
        "flex items-center justify-center group",
      )}
      onMouseDown={onMouseDown}
      onDoubleClick={onDoubleClick}
      tabIndex={-1}
    >
      <div
        className={cn(
          "w-12 h-1 rounded-full transition-all group-hover:h-1.5 group-hover:w-14",
          getHandleColor(planModeEnabled, isAgentBusy, isStarting),
        )}
      />
    </button>
  );
}
