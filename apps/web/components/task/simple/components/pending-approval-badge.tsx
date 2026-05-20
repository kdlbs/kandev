"use client";

import { useMemo } from "react";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { Task, TaskDecision } from "@/app/office/tasks/[id]/types";

// computePendingApprovers returns the names of approvers who have not
// recorded an active "approved" decision. Used to render the gated
// status badge tooltip when the task sits in `in_review`.
export function computePendingApprovers(
  task: Pick<Task, "approvers" | "decisions">,
  agentNames: Record<string, string>,
): string[] {
  const approvedIds = new Set(
    task.decisions
      .filter((d) => d.role === "approver" && d.decision === "approved")
      .map((d) => d.deciderId),
  );
  return task.approvers.filter((id) => !approvedIds.has(id)).map((id) => agentNames[id] ?? id);
}

// pickActiveDecisions reduces a decisions list to the most recent
// active (non-superseded) decision per (decider, role). The server
// already only sends active rows, but this helper is exported so
// component tests can reuse the same selector logic.
export function pickActiveDecisions(decisions: TaskDecision[]): TaskDecision[] {
  const seen = new Map<string, TaskDecision>();
  for (const d of decisions) {
    const key = `${d.deciderType}:${d.deciderId}:${d.role}`;
    const prev = seen.get(key);
    if (!prev || prev.createdAt < d.createdAt) seen.set(key, d);
  }
  return Array.from(seen.values());
}

type PendingApprovalBadgeProps = {
  task: Task;
};

export function PendingApprovalBadge({ task }: PendingApprovalBadgeProps) {
  const agents = useAppStore((s) => s.office.agentProfiles);
  const agentLookup = useMemo(() => {
    const map: Record<string, string> = {};
    for (const a of agents) map[a.id] = a.name;
    return map;
  }, [agents]);

  const pending = useMemo(() => computePendingApprovers(task, agentLookup), [task, agentLookup]);

  if (task.status !== "in_review" || pending.length === 0) return null;

  const label = `Awaiting approval from ${pending.length} agent${pending.length === 1 ? "" : "s"}`;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          className="text-xs cursor-help"
          data-testid="pending-approval-badge"
        >
          {label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent data-testid="pending-approval-tooltip">{pending.join(", ")}</TooltipContent>
    </Tooltip>
  );
}
