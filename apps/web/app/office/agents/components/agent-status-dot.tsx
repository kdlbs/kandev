"use client";

import { useQuery } from "@tanstack/react-query";
import { cn } from "@/lib/utils";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { AgentStatus } from "@/lib/state/slices/office/types";

const FALLBACK_STYLES: Record<AgentStatus, string> = {
  idle: "bg-neutral-400",
  working: "bg-cyan-400 animate-pulse",
  paused: "bg-yellow-400",
  stopped: "bg-neutral-400 opacity-50",
  pending_approval: "bg-orange-400",
};

type AgentStatusDotProps = {
  status: AgentStatus;
  className?: string;
};

export function AgentStatusDot({ status, className }: AgentStatusDotProps) {
  const { data: meta } = useQuery(officeQueryOptions.metaGlobal());
  const metaStatus = meta?.agentStatuses.find((s) => s.id === status);
  const colorClass = metaStatus?.color ?? FALLBACK_STYLES[status] ?? "";
  const label = metaStatus?.label ?? status;
  return (
    <span
      className={cn("inline-block h-2 w-2 rounded-full shrink-0", colorClass, className)}
      title={label}
    />
  );
}
