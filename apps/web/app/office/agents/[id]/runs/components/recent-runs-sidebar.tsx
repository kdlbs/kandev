"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { IconLoader2 } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { cn } from "@/lib/utils";
import type { AgentRunSummary } from "@/lib/api/domains/office-extended-api";
import { timeAgo } from "../../../../components/shared/time-ago";

type Props = {
  runs: AgentRunSummary[];
  agentId: string;
  /**
   * Fallback active run id used during SSR and tests where
   * `usePathname()` may not yet have the resolved client URL. The
   * client-side render derives the active id from the pathname so
   * navigating between sibling runs updates the highlight without a
   * server round-trip.
   */
  activeRunId: string;
};

const RUN_PATH_RE = /\/runs\/([^/?#]+)/;

function deriveActiveRunId(pathname: string | null, fallback: string): string {
  if (!pathname) return fallback;
  const match = RUN_PATH_RE.exec(pathname);
  return match?.[1] ?? fallback;
}

/**
 * Static recent-runs sidebar rendered alongside the run detail.
 *
 * - Active row is derived from `usePathname()` so client-side
 *   navigation between sibling runs updates the highlight without
 *   re-rendering the parent.
 * - In-progress (`claimed`) rows show an animated spinner icon.
 * - Each row is a Next.js `<Link>` with `aria-current="page"` on the
 *   active one so it's keyboard-navigable + screen-reader friendly.
 */
export function RecentRunsSidebar({ runs, agentId, activeRunId }: Props) {
  const pathname = usePathname();
  const currentId = deriveActiveRunId(pathname, activeRunId);

  if (runs.length === 0) {
    return <div className="text-xs text-muted-foreground p-4">No runs yet.</div>;
  }
  return (
    <nav
      className="flex flex-col divide-y divide-border border border-border rounded-lg overflow-hidden"
      aria-label="Recent runs"
      data-testid="recent-runs-sidebar"
    >
      {runs.map((run) => (
        <RecentRunsRow key={run.id} run={run} agentId={agentId} active={run.id === currentId} />
      ))}
    </nav>
  );
}

type RowProps = {
  run: AgentRunSummary;
  agentId: string;
  active: boolean;
};

function RecentRunsRow({ run, agentId, active }: RowProps) {
  const isRunning = run.status === "claimed";
  return (
    <Link
      href={`/office/agents/${agentId}/runs/${run.id}`}
      aria-current={active ? "page" : undefined}
      className={cn(
        "relative px-3 py-2 text-xs hover:bg-muted/50 transition-colors cursor-pointer",
        active && "bg-muted font-medium",
        active &&
          "before:absolute before:left-0 before:top-0 before:bottom-0 before:w-0.5 before:bg-primary",
      )}
      data-testid={`recent-runs-row-${run.id}`}
      data-active={active ? "true" : "false"}
    >
      <div className="flex items-center gap-2">
        {isRunning && (
          <IconLoader2
            className="h-3 w-3 animate-spin text-primary shrink-0"
            data-testid={`recent-run-row-running-icon-${run.id}`}
            aria-hidden="true"
          />
        )}
        <span className="font-mono text-[11px]">{run.id_short}</span>
        <Badge variant="outline" className="text-[10px]">
          {run.status}
        </Badge>
        <span className="ml-auto text-[10px] text-muted-foreground">
          {timeAgo(run.requested_at)}
        </span>
      </div>
      <div className="text-[11px] text-muted-foreground truncate mt-0.5">{run.reason}</div>
    </Link>
  );
}
