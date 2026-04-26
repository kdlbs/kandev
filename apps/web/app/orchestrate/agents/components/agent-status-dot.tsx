import { cn } from "@/lib/utils";
import type { AgentStatus } from "@/lib/state/slices/orchestrate/types";

const statusStyles: Record<AgentStatus, string> = {
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
  return (
    <span
      className={cn("inline-block h-2 w-2 rounded-full shrink-0", statusStyles[status], className)}
      title={status}
    />
  );
}
