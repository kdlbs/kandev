"use client";

import { cn } from "@/lib/utils";
import { CompositorPulse } from "@kandev/ui/compositor-pulse";
import { useAppStore } from "@/components/state-provider";
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
  const meta = useAppStore((s) => s.office.meta);
  const metaStatus = meta?.agentStatuses.find((s) => s.id === status);
  const colorClass = metaStatus?.color ?? FALLBACK_STYLES[status] ?? "";
  // Workspace metadata owns this label. The `?? status` fallback only fires
  // before `office.meta` hydrates and renders the raw wire value, which is an
  // identifier rather than copy — translating it would invent a label the rest
  // of the app never shows.
  const label = metaStatus?.label ?? status;
  const Dot = status === "working" ? CompositorPulse : "span";
  return (
    <Dot
      className={cn("inline-block h-2 w-2 rounded-full shrink-0", colorClass, className)}
      title={label}
    />
  );
}
