"use client";

import { GridSpinner } from "@/components/grid-spinner";
import { cn } from "@/lib/utils";

type PanelLoadingStateProps = {
  label: string;
  /** Override the outer container layout (e.g. absolute overlay for the terminal). */
  className?: string;
  testId?: string;
};

/**
 * Shared panel loading placeholder: a grid spinner above a label, matching the
 * terminal's "Connecting terminal..." look. Used by the file browser, terminal,
 * and agent preview panels so loading states stay visually consistent.
 */
export function PanelLoadingState({ label, className, testId }: PanelLoadingStateProps) {
  return (
    <div
      data-testid={testId}
      className={cn("flex h-full w-full items-start justify-center pt-12", className)}
    >
      <div className="flex flex-col items-center gap-3 text-muted-foreground">
        <GridSpinner />
        <span className="text-sm">{label}</span>
      </div>
    </div>
  );
}
