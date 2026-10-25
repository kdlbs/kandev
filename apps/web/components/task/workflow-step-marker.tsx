"use client";

import { IconLoader2 } from "@tabler/icons-react";

export type StepMarkerState = "current" | "completed" | "upcoming" | "pending";

export function StepCircleIndicator({
  isCurrent,
  isCompleted,
  isPending = false,
  pendingLabel,
}: {
  isCurrent: boolean;
  isCompleted: boolean;
  isPending?: boolean;
  pendingLabel?: string;
}) {
  let state: StepMarkerState = "upcoming";
  if (isPending) state = "pending";
  else if (isCurrent) state = "current";
  else if (isCompleted) state = "completed";
  if (isPending) {
    return (
      <span
        data-marker-state={state}
        role={pendingLabel ? "img" : undefined}
        aria-label={pendingLabel}
        className="relative flex h-2 w-2 items-center justify-center shrink-0"
      >
        <IconLoader2
          aria-hidden="true"
          data-marker-visual-size={isCurrent ? "14" : "8"}
          className={[
            "absolute animate-spin text-primary motion-reduce:animate-none",
            isCurrent ? "h-3.5 w-3.5" : "h-2 w-2",
          ].join(" ")}
        />
      </span>
    );
  }
  if (isCurrent) {
    return (
      <span
        data-marker-state={state}
        className="relative flex items-center justify-center shrink-0"
      >
        <span className="absolute h-3.5 w-3.5 rounded-full border-2 border-primary/40" />
        <span className="h-2 w-2 rounded-full bg-primary" />
      </span>
    );
  }
  if (isCompleted) {
    return (
      <span
        data-marker-state={state}
        className="relative flex items-center justify-center shrink-0"
      >
        <span className="h-2 w-2 rounded-full bg-muted-foreground/60" />
      </span>
    );
  }
  return (
    <span data-marker-state={state} className="relative flex items-center justify-center shrink-0">
      <span className="h-2 w-2 rounded-full border border-muted-foreground/40" />
    </span>
  );
}
