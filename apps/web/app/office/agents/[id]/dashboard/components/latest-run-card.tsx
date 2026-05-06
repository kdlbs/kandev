import Link from "next/link";
import { IconChevronRight } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent } from "@kandev/ui/card";
import { cn } from "@/lib/utils";
import type { AgentLatestRun } from "@/lib/api/domains/office-extended-api";
import { formatShortDate } from "./format-date";

type Props = {
  run: AgentLatestRun | null;
  agentId: string;
};

/**
 * Visual style per run status. Backend statuses come straight from
 * office_runs.status: queued | claimed | finished | failed |
 * cancelled (+ a synthetic `scheduled_retry` follow-up).
 */
function statusBadgeClass(status: string): string {
  switch (status) {
    case "finished":
      return "bg-emerald-500/15 text-emerald-700 border-emerald-500/30";
    case "failed":
      return "bg-red-500/15 text-red-700 border-red-500/30";
    case "claimed":
      return "bg-blue-500/15 text-blue-700 border-blue-500/30";
    case "cancelled":
      return "bg-muted text-muted-foreground border-muted-foreground/30";
    default:
      return "bg-amber-500/15 text-amber-700 border-amber-500/30";
  }
}

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

export function LatestRunCard({ run, agentId }: Props) {
  if (!run) {
    return (
      <Card data-testid="latest-run-card">
        <CardContent className="py-4">
          <div className="flex items-center justify-between gap-2">
            <span className="text-sm font-medium">Latest run</span>
            <span className="text-xs text-muted-foreground">No runs yet</span>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card
      data-testid="latest-run-card"
      data-run-id={run.run_id}
      className="hover:bg-muted/30 transition-colors"
    >
      <Link
        href={`/office/agents/${agentId}/runs/${run.run_id}`}
        className="block cursor-pointer"
        data-testid="latest-run-link"
      >
        <CardContent className="py-3 space-y-1.5">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">Latest run</span>
            <Badge
              variant="outline"
              className={cn("text-[11px] py-0", statusBadgeClass(run.status))}
              data-testid="latest-run-status"
            >
              {run.status}
            </Badge>
            <span
              data-testid="latest-run-requested-at"
              className="ml-auto text-xs text-muted-foreground"
            >
              {formatShortDate(run.requested_at)}
            </span>
            <IconChevronRight className="h-4 w-4 text-muted-foreground" aria-hidden />
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span className="font-mono text-foreground">{run.run_id_short}</span>
            <span aria-hidden>·</span>
            <span>{formatReason(run.reason)}</span>
          </div>
          {run.summary ? (
            <p className="text-sm text-muted-foreground line-clamp-2">{run.summary}</p>
          ) : null}
        </CardContent>
      </Link>
    </Card>
  );
}
