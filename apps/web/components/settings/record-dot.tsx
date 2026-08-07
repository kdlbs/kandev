"use client";

import { cn } from "@/lib/utils";

/**
 * The mark in front of a record — an agent profile on the Agents page, and the
 * same profile (plus workspaces and executor profiles) in the settings menu.
 *
 * For a record that can be turned off, the mark carries that state. A disabled
 * agent profile is hidden from every task and session picker while staying
 * listed and editable here, and until now nothing in either list said so — the
 * toggle lives inside the profile's own page, so you had to open a profile to
 * learn it was off.
 *
 * Disabled is drawn hollow *and* dimmer, not dimmer alone: state carried by
 * colour by itself is invisible to a colour-blind reader, and this dot is the
 * only thing marking it in both lists.
 */
export function RecordDot({
  enabled,
  className,
}: {
  /** Omitted for records with no such concept — a workspace is never "off". */
  enabled?: boolean;
  className?: string;
}) {
  const disabled = enabled === false;
  return (
    <span
      aria-hidden
      data-record-dot={disabled ? "disabled" : "enabled"}
      className={cn(
        "h-1.5 w-1.5 shrink-0 rounded-full",
        disabled ? "border border-muted-foreground/70" : "bg-primary/70",
        className,
      )}
    />
  );
}
