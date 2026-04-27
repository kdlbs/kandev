"use client";

import { useCallback, useEffect, useState } from "react";
import { IconClock, IconRun } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { listWakeups } from "@/lib/api/domains/orchestrate-api";
import type { AgentInstance, WakeupEntry } from "@/lib/state/slices/orchestrate/types";
import { timeAgo } from "../../../components/shared/time-ago";

type AgentRunsTabProps = {
  agent: AgentInstance;
};

const STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  finished: "default",
  claimed: "secondary",
  queued: "outline",
  failed: "destructive",
};

export function AgentRunsTab({ agent }: AgentRunsTabProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [runs, setRuns] = useState<WakeupEntry[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchRuns = useCallback(async () => {
    if (!workspaceId) return;
    try {
      const res = await listWakeups(workspaceId);
      const agentRuns = (res.wakeups ?? []).filter(
        (w) => w.agentInstanceId === agent.id,
      );
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
      <div className="grid grid-cols-[1fr_100px_140px] gap-4 px-4 py-2 text-xs font-medium text-muted-foreground uppercase tracking-wider">
        <span>Reason</span>
        <span>Status</span>
        <span>Requested</span>
      </div>
      {runs.map((run) => (
        <div
          key={run.id}
          className="grid grid-cols-[1fr_100px_140px] gap-4 px-4 py-2.5 text-sm"
        >
          <span className="truncate">{run.reason}</span>
          <Badge variant={STATUS_VARIANT[run.status] ?? "secondary"}>
            {run.status}
          </Badge>
          <span className="text-xs text-muted-foreground flex items-center gap-1">
            <IconClock className="h-3.5 w-3.5" />
            {timeAgo(run.requestedAt)}
          </span>
        </div>
      ))}
    </div>
  );
}
