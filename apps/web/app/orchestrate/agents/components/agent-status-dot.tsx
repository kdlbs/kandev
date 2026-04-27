"use client";

import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { AgentStatus } from "@/lib/state/slices/orchestrate/types";

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
  const meta = useAppStore((s) => s.orchestrate.meta);
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
