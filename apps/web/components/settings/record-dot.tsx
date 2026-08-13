"use client";

import { cn } from "@/lib/utils";

/**
 * The mark in front of a record — an agent profile on the Agents page, and the
 * same profile, plus workspaces and executor profiles, in the settings menu.
 *
 * One static appearance for every record. It says "this one is yours", nothing
 * more: a mark that also carried state would mean two different things
 * depending on which kind of record it sat in front of.
 */
export function RecordDot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden
      data-record-dot=""
      className={cn("h-1.5 w-1.5 shrink-0 rounded-full bg-primary/70", className)}
    />
  );
}
