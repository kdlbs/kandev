"use client";

import { useCallback, useEffect, useState } from "react";
import { IconClock, IconRun } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { listRuns } from "@/lib/api/domains/office-api";
import type { AgentProfile, Run } from "@/lib/state/slices/office/types";
import { timeAgo } from "../../../components/shared/time-ago";

type AgentRunsTabProps = {
  agent: AgentProfile;
};

const STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  finished: "default",
  claimed: "secondary",
  queued: "outline",
  failed: "destructive",
  cancelled: "outline",
};

const CANCEL_REASON_LABEL: Record<string, string> = {
  assignee_changed: "Assignee changed",
  task_terminal: "Task completed",
  task_not_found: "Task not found",
  review_participant_changed: "Reviewer changed",
  retry_stale_assignee: "Retry stale",
  retry_task_cancelled: "Task cancelled",
};

const CANCEL_REASON_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  assignee_changed: "destructive",
  task_terminal: "secondary",
  task_not_found: "secondary",
  review_participant_changed: "outline",
  retry_stale_assignee: "outline",
  retry_task_cancelled: "secondary",
};

function truncate(s: string, max: number): string {
  if (s.length <= max) return s;
  return s.slice(0, max) + "…";
}

function CancelReasonBadge({ reason }: { reason: string }) {
  const label = CANCEL_REASON_LABEL[reason] ?? reason.replace(/_/g, " ");
  const variant = CANCEL_REASON_VARIANT[reason] ?? "outline";
  return (
    <Badge variant={variant} className="ml-1 text-[10px]">
      {label}
    </Badge>
  );
}

export function AgentRunsTab({ agent }: AgentRunsTabProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [runs, setRuns] = useState<Run[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchRuns = useCallback(async () => {
    if (!workspaceId) return;
    try {
      const res = await listRuns(workspaceId);
      const agentRuns = (res.runs ?? []).filter((w) => w.agent_profile_id === agent.id);
      setRuns(agentRuns);
    } catch {
      // Silently handle - empty state will show
    } finally {
      setLoading(false);
    }
  }, [workspaceId, agent.id]);

  useEffect(() => {
    void fetchRuns();
  }, [fetchRuns]);

  // Refresh runs reactively when runs change. The office WS handler
  // triggers "runs" on office.run.queued and the agent-session
  // path triggers "agents" on session.state_changed.
  useOfficeRefetch("runs", fetchRuns);
  useOfficeRefetch("agents", fetchRuns);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <p className="text-sm text-muted-foreground">Loading runs...</p>
      </div>
    );
  }

  if (runs.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <IconRun className="h-10 w-10 text-muted-foreground/30 mb-3" />
        <p className="text-sm text-muted-foreground">No runs yet.</p>
        <p className="text-xs text-muted-foreground mt-1">
          Assign a task to this agent to see execution history.
        </p>
      </div>
    );
  }

  return (
    <div className="mt-4 border border-border rounded-lg divide-y divide-border">
      <div className="grid grid-cols-[1fr_160px_140px] gap-4 px-4 py-2 text-xs font-medium text-muted-foreground uppercase tracking-wider">
        <span>Reason</span>
        <span>Status</span>
        <span>Requested</span>
      </div>
      {runs.map((run) => (
        <div key={run.id} className="grid grid-cols-[1fr_160px_140px] gap-4 px-4 py-2.5 text-sm">
          <span className="truncate">{run.reason}</span>
          <span
            className="flex items-center flex-wrap gap-1"
            title={run.error_message ?? undefined}
          >
            <Badge variant={STATUS_VARIANT[run.status] ?? "secondary"}>{run.status}</Badge>
            {run.status === "cancelled" && run.cancel_reason && (
              <CancelReasonBadge reason={run.cancel_reason} />
            )}
            {run.status === "failed" && run.error_message && (
              <span className="text-xs text-muted-foreground truncate max-w-[120px]">
                {truncate(run.error_message, 60)}
              </span>
            )}
          </span>
          <span className="text-xs text-muted-foreground flex items-center gap-1">
            <IconClock className="h-3.5 w-3.5" />
            {timeAgo(run.requested_at)}
          </span>
        </div>
      ))}
    </div>
  );
}
