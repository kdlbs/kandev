"use client";

import { useCallback, useRef, useState } from "react";
import Link from "next/link";
import {
  IconClock,
  IconLoader2,
  IconMessageCircle,
  IconRepeat,
  IconRun,
  IconChecklist,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import {
  listAgentRuns,
  type AgentRunsListPage,
  type AgentRunSummary,
} from "@/lib/api/domains/office-extended-api";
import { timeAgo } from "../../../components/shared/time-ago";

type Props = {
  initial: AgentRunsListPage;
  agentId: string;
};

const STATUS_VARIANT: Record<
  AgentRunSummary["status"],
  "default" | "secondary" | "destructive" | "outline"
> = {
  finished: "default",
  claimed: "secondary",
  queued: "outline",
  failed: "destructive",
  cancelled: "outline",
};

/**
 * Pretty-prints a run reason like `task_assigned` →
 * `Task assigned`. Reasons are stable enum strings on the backend,
 * so a generic transformer is enough.
 */
function formatReason(reason: string): string {
  if (!reason) return "—";
  const text = reason.replaceAll("_", " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/**
 * Client-side runs list with cursor-based "Load more" pagination.
 * The Server Component delivers page 1; subsequent pages are
 * fetched via `listAgentRuns` and appended to a flat array. Scroll
 * position is preserved when a new page is appended.
 */
export function RunsListView({ initial, agentId }: Props) {
  const [pages, setPages] = useState<AgentRunsListPage[]>([initial]);
  const [loading, setLoading] = useState(false);
  const containerRef = useRef<HTMLDivElement | null>(null);

  const lastPage = pages[pages.length - 1];
  const runs = pages.flatMap((p) => p.runs);
  const hasMore = Boolean(lastPage?.next_cursor);

  const loadMore = useCallback(async () => {
    if (!hasMore || loading) return;
    const scrollY = containerRef.current?.scrollTop ?? 0;
    setLoading(true);
    try {
      const next = await listAgentRuns(agentId, {
        cursor: lastPage.next_cursor,
        cursorId: lastPage.next_id,
        limit: 25,
      });
      setPages((prev) => [...prev, next]);
      requestAnimationFrame(() => {
        if (containerRef.current) {
          containerRef.current.scrollTop = scrollY;
        }
      });
    } finally {
      setLoading(false);
    }
  }, [agentId, hasMore, lastPage, loading]);

  if (runs.length === 0) {
    return (
      <div
        className="flex flex-col items-center justify-center py-12 text-center"
        data-testid="agent-runs-empty"
      >
        <IconRun className="h-10 w-10 text-muted-foreground/30 mb-3" />
        <p className="text-sm text-muted-foreground">No runs yet.</p>
        <p className="text-xs text-muted-foreground mt-1">
          Assign a task to this agent to see execution history.
        </p>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      className="mt-4 border border-border rounded-lg divide-y divide-border"
      data-testid="agent-runs-list"
    >
      <div className="grid grid-cols-[120px_140px_1fr_120px_120px] gap-4 px-4 py-2 text-xs font-medium text-muted-foreground uppercase tracking-wider">
        <span>Run</span>
        <span>Reason</span>
        <span>Linked</span>
        <span>Status</span>
        <span>Requested</span>
      </div>
      {runs.map((run) => (
        <RunRow key={run.id} run={run} agentId={agentId} />
      ))}
      <LoadMoreFooter hasMore={hasMore} loading={loading} onLoadMore={loadMore} />
    </div>
  );
}

function RunRow({ run, agentId }: { run: AgentRunSummary; agentId: string }) {
  return (
    <div
      className="grid grid-cols-[120px_140px_1fr_120px_120px] gap-4 px-4 py-2.5 text-sm hover:bg-muted/30 transition-colors"
      data-testid={`agent-run-row-${run.id}`}
    >
      <Link
        href={`/office/agents/${agentId}/runs/${run.id}`}
        className="font-mono text-xs hover:underline cursor-pointer"
      >
        {run.id_short}
      </Link>
      <Link
        href={`/office/agents/${agentId}/runs/${run.id}`}
        className="truncate cursor-pointer hover:underline"
      >
        {formatReason(run.reason)}
      </Link>
      <LinkedEntity run={run} />
      <span>
        <Badge variant={STATUS_VARIANT[run.status] ?? "secondary"}>{run.status}</Badge>
      </span>
      <span className="text-xs text-muted-foreground flex items-center gap-1">
        <IconClock className="h-3.5 w-3.5" />
        {timeAgo(run.requested_at)}
      </span>
    </div>
  );
}

/**
 * Renders the cell that deeplinks the row to its triggering entity:
 * a routine (cron fire), a task comment (user reply), or a task
 * (assignment). Falls back to em-dash for runs with no linkable
 * origin (legacy rows, scheduled wakeups without a task).
 */
function LinkedEntity({ run }: { run: AgentRunSummary }) {
  const task = useAppStore((s) =>
    run.task_id ? s.office.tasks.items.find((t) => t.id === run.task_id) : undefined,
  );
  const routine = useAppStore((s) =>
    run.routine_id ? s.office.routines.find((r) => r.id === run.routine_id) : undefined,
  );

  if (run.routine_id) {
    const label = routine?.name ?? "Routine";
    return (
      <Link
        href={`/office/routines/${run.routine_id}`}
        className="flex items-center gap-1.5 text-xs hover:underline cursor-pointer min-w-0"
        title={label}
      >
        <IconRepeat className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{label}</span>
      </Link>
    );
  }

  if (run.task_id && run.comment_id) {
    const label = task ? `${task.identifier}: ${task.title}` : "Comment";
    return (
      <Link
        href={`/office/tasks/${run.task_id}#comment-${run.comment_id}`}
        className="flex items-center gap-1.5 text-xs hover:underline cursor-pointer min-w-0"
        title={label}
      >
        <IconMessageCircle className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{label}</span>
      </Link>
    );
  }

  if (run.task_id) {
    const label = task ? `${task.identifier}: ${task.title}` : "Task";
    return (
      <Link
        href={`/office/tasks/${run.task_id}`}
        className="flex items-center gap-1.5 text-xs hover:underline cursor-pointer min-w-0"
        title={label}
      >
        <IconChecklist className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{label}</span>
      </Link>
    );
  }

  return <span className="text-xs text-muted-foreground">—</span>;
}

type LoadMoreFooterProps = {
  hasMore: boolean;
  loading: boolean;
  onLoadMore: () => void;
};

function LoadMoreFooter({ hasMore, loading, onLoadMore }: LoadMoreFooterProps) {
  if (hasMore) {
    return (
      <div className="flex items-center justify-center py-3">
        <Button
          variant="outline"
          size="sm"
          onClick={onLoadMore}
          disabled={loading}
          className="cursor-pointer gap-1.5"
          data-testid="agent-runs-load-more"
          aria-busy={loading || undefined}
        >
          {loading && (
            <IconLoader2
              className="h-3.5 w-3.5 animate-spin"
              data-testid="agent-runs-load-more-spinner"
              aria-hidden="true"
            />
          )}
          {loading ? "Loading…" : "Load more"}
        </Button>
      </div>
    );
  }
  return (
    <div
      className="flex items-center justify-center py-3 text-muted-foreground text-xs"
      data-testid="agent-runs-end-of-list"
    >
      No more runs
    </div>
  );
}
