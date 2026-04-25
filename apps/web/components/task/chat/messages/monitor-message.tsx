"use client";

import { memo } from "react";
import { IconActivity, IconCheck, IconX } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { readMonitorView } from "@/components/task/chat/types";
import { ExpandableRow } from "./expandable-row";
import { useExpandState } from "./use-expand-state";

type MonitorMessageProps = {
  comment: Message;
};

// Status icon mirroring the other tool cards: spinner while watching,
// checkmark when the parent prompt completes naturally, X when the agent
// process restarted (the adapter sets status=cancelled in that case).
function MonitorStatusIcon({ status, ended }: { status: string | undefined; ended: boolean }) {
  if (!ended) return <GridSpinner className="text-muted-foreground" />;
  if (status === "cancelled") return <IconX className="h-3.5 w-3.5 text-amber-500" />;
  return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
}

function formatStatusLabel(
  status: string | undefined,
  ended: boolean,
  endReason: string | undefined,
) {
  if (!ended) return "watching";
  if (status === "cancelled" || endReason === "session_restart") return "ended (session restart)";
  return "ended";
}

type MonitorViewModel = {
  status: ToolCallMetadata["status"];
  ended: boolean;
  endReason: string;
  eventCount: number;
  recentEvents: string[];
  title: string;
  countSuffix: string;
};

function pluralizeEventCount(n: number): string {
  if (n <= 0) return "";
  return ` · ${n} event${n === 1 ? "" : "s"}`;
}

function buildMonitorViewModel(comment: Message): MonitorViewModel {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const view = readMonitorView(metadata?.normalized?.generic) ?? {};
  const command = view.command ?? "";
  const eventCount = view.event_count ?? 0;
  return {
    status: metadata?.status,
    ended: view.ended === true,
    endReason: view.end_reason ?? "",
    eventCount,
    recentEvents: view.recent_events ?? [],
    title: command ? `Monitor: ${command}` : "Monitor",
    countSuffix: pluralizeEventCount(eventCount),
  };
}

function MonitorHeader({ vm }: { vm: MonitorViewModel }) {
  return (
    <div className="flex items-center gap-2 text-xs" data-testid="monitor-card">
      <span className="inline-flex items-center gap-1.5">
        <span className="font-mono text-xs text-muted-foreground">{vm.title}</span>
        {vm.countSuffix && (
          <span className="text-xs text-muted-foreground/80" data-testid="monitor-event-count">
            {vm.countSuffix}
          </span>
        )}
        <span
          className={cn(
            "text-[10px] uppercase tracking-wide rounded px-1.5 py-0.5",
            vm.ended
              ? "bg-muted text-muted-foreground"
              : "bg-blue-500/10 text-blue-700 dark:text-blue-400",
          )}
          data-testid="monitor-status-pill"
        >
          {formatStatusLabel(vm.status, vm.ended, vm.endReason)}
        </span>
        <MonitorStatusIcon status={vm.status} ended={vm.ended} />
      </span>
    </div>
  );
}

export const MonitorMessage = memo(function MonitorMessage({ comment }: MonitorMessageProps) {
  const vm = buildMonitorViewModel(comment);
  // Auto-expand whenever there are events to surface — both during the
  // watch (so users see the stream live) and after it ends (so they can
  // review what happened). Empty Monitors collapse since there's nothing
  // to show.
  const { isExpanded, handleToggle } = useExpandState(vm.status, vm.recentEvents.length > 0);

  return (
    <ExpandableRow
      icon={
        <IconActivity
          className={cn("h-4 w-4", vm.ended ? "text-muted-foreground" : "text-blue-500")}
          data-testid="monitor-card-icon"
        />
      }
      header={<MonitorHeader vm={vm} />}
      hasExpandableContent={vm.recentEvents.length > 0}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <MonitorEventList events={vm.recentEvents} />
    </ExpandableRow>
  );
});

function MonitorEventList({ events }: { events: string[] }) {
  if (events.length === 0) return null;
  return (
    <div className="pl-4 border-l-2 border-border/30 space-y-1" data-testid="monitor-event-list">
      {events.map((line, idx) => (
        <pre
          key={`${idx}-${line.slice(0, 32)}`}
          className="text-xs bg-muted/30 rounded p-2 overflow-x-auto whitespace-pre-wrap"
          data-testid="monitor-event"
        >
          {line}
        </pre>
      ))}
    </div>
  );
}
