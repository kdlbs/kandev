"use client";

import { cn } from "@/lib/utils";
import type { RunEvent } from "@/lib/api/domains/office-extended-api";

type Props = {
  events: RunEvent[];
};

const LEVEL_COLOR: Record<string, string> = {
  info: "text-muted-foreground",
  warn: "text-amber-600 dark:text-amber-400",
  warning: "text-amber-600 dark:text-amber-400",
  error: "text-red-600 dark:text-red-400",
  debug: "text-muted-foreground/70",
};

/**
 * Events log — renders the structured run events emitted by the
 * orchestrator (init / adapter.invoke / step / complete / error).
 * Level → color mapping is static; payload is shown verbatim as JSON
 * for now, since structured rendering is per-event-type and lands as
 * Wave 2.E polish.
 */
export function EventsLog({ events }: Props) {
  if (events.length === 0) {
    return (
      <div
        className="rounded-lg border border-border p-4 text-xs text-muted-foreground"
        data-testid="events-log-empty"
      >
        No events recorded for this run yet.
      </div>
    );
  }
  return (
    <div className="rounded-lg border border-border" data-testid="events-log">
      <div className="px-4 py-2 border-b border-border text-xs font-medium uppercase tracking-wider text-muted-foreground">
        Events
      </div>
      <ul className="divide-y divide-border max-h-[400px] overflow-y-auto">
        {events.map((ev) => (
          <li
            key={ev.seq}
            className="px-4 py-2 text-xs grid grid-cols-[80px_120px_1fr] gap-3 items-baseline"
            data-testid={`events-log-row-${ev.seq}`}
          >
            <span className={cn("font-medium", LEVEL_COLOR[ev.level] ?? "")}>{ev.event_type}</span>
            <span className="text-muted-foreground font-mono">{ev.created_at}</span>
            <span className="font-mono text-muted-foreground/80 break-all">
              {ev.payload && ev.payload !== "{}" ? ev.payload : ""}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
