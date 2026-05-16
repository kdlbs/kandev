"use client";

import {
  IconAlertTriangle,
  IconClock,
  IconCoin,
  IconPlayerStop,
  IconRefresh,
  IconPlayerPlay,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { RunDetail } from "@/lib/api/domains/office-extended-api";
import { formatDollars } from "@/lib/utils";

type Props = {
  run: RunDetail;
};

const STATUS_VARIANT: Record<
  RunDetail["status"],
  "default" | "secondary" | "destructive" | "outline"
> = {
  finished: "default",
  claimed: "secondary",
  queued: "outline",
  failed: "destructive",
  cancelled: "outline",
};

function formatDuration(ms?: number): string {
  if (!ms || ms <= 0) return "—";
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  const rem = sec % 60;
  return rem === 0 ? `${min}m` : `${min}m ${rem}s`;
}

// formatCostSubcents renders an int64 subcents (hundredths of a cent)
// value as a USD dollar string. Locally defined wrapper around the
// shared formatDollars helper so this file's import surface stays
// the same after the office-costs cost_cents -> cost_subcents rename.
function formatCostSubcents(subcents: number): string {
  if (subcents === 0) return "$0.00";
  return formatDollars(subcents);
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

/**
 * Header strip for the run detail page. Shows the status badge,
 * adapter family + model, time range, duration, token + cost
 * summary, and the action buttons appropriate to the run's state
 * (Cancel / Resume / Start fresh). Recover actions wire to the
 * existing `session.recover` WS request, matching the chat
 * RunErrorEntry behaviour exactly.
 */
export function RunHeader({ run }: Props) {
  const isFailed = run.status === "failed";
  const isRunning = run.status === "claimed";
  const sessionId = run.session.session_id;
  return (
    <div className="rounded-lg border border-border p-4 space-y-3" data-testid="run-header">
      <TopRow run={run} />
      <RoutingStrip routing={run.routing} />
      <StatsGrid run={run} />
      <TokensRow run={run} />
      {isFailed && run.error_message && <ErrorBanner message={run.error_message} />}
      <ActionBar
        runId={run.task_id}
        sessionId={sessionId}
        isRunning={isRunning}
        isFailed={isFailed}
      />
    </div>
  );
}

// RoutingStrip renders a concise summary of the run's routing decision.
// Only present when the run went through the routing path. The full
// per-attempt panel lives in route-attempt-list and is not duplicated
// here; this strip just calls out resolved vs intended + block state.
function RoutingStrip({ routing }: { routing?: RunDetail["routing"] }) {
  if (!routing) return null;
  const resolved = routing.resolved_provider_id ?? "";
  const intended = routing.logical_provider_order?.[0] ?? "";
  const fellBack = resolved !== "" && intended !== "" && resolved !== intended;
  const blocked = routing.blocked_status ?? "";
  return (
    <div className="space-y-1 text-xs" data-testid="run-routing-strip">
      {resolved !== "" && (
        <div className="flex items-center gap-2 flex-wrap" data-testid="run-routing-resolved">
          <span className="text-muted-foreground uppercase tracking-wide">Resolved</span>
          <span className="font-mono">
            {resolved}
            {routing.resolved_model ? ` · ${routing.resolved_model}` : ""}
          </span>
        </div>
      )}
      {fellBack && (
        <div className="flex items-center gap-2 flex-wrap" data-testid="run-routing-intended">
          <span className="text-muted-foreground uppercase tracking-wide">Intended</span>
          <span className="font-mono">{intended}</span>
        </div>
      )}
      {blocked !== "" && <RoutingBlockBadge status={blocked} retryAt={routing.earliest_retry_at} />}
    </div>
  );
}

function RoutingBlockBadge({ status, retryAt }: { status: string; retryAt?: string }) {
  if (status === "blocked_provider_action_required") {
    return (
      <Badge variant="destructive" data-testid="run-routing-blocked">
        Blocked — action required
      </Badge>
    );
  }
  if (status === "waiting_for_provider_capacity") {
    return (
      <Badge variant="secondary" data-testid="run-routing-waiting">
        Waiting for capacity{retryAt ? ` · retry ${retryAt}` : ""}
      </Badge>
    );
  }
  return null;
}

function TopRow({ run }: { run: RunDetail }) {
  return (
    <div className="flex items-center gap-3 flex-wrap">
      <Badge variant={STATUS_VARIANT[run.status] ?? "secondary"} data-testid="run-status-badge">
        {run.status}
      </Badge>
      {run.invocation.adapter && (
        <span className="text-sm text-muted-foreground" data-testid="run-adapter">
          {run.invocation.adapter}
          {run.invocation.model ? ` · ${run.invocation.model}` : ""}
        </span>
      )}
      <span className="text-xs text-muted-foreground font-mono ml-auto">{run.id_short}</span>
    </div>
  );
}

function StatsGrid({ run }: { run: RunDetail }) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
      <div>
        <div className="text-xs text-muted-foreground uppercase tracking-wider">Started</div>
        <div data-testid="run-started-at">{run.claimed_at || run.requested_at}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground uppercase tracking-wider">Finished</div>
        <div data-testid="run-finished-at">{run.finished_at || "—"}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground uppercase tracking-wider flex items-center gap-1">
          <IconClock className="h-3 w-3" /> Duration
        </div>
        <div data-testid="run-duration">{formatDuration(run.duration_ms)}</div>
      </div>
      <div>
        <div className="text-xs text-muted-foreground uppercase tracking-wider flex items-center gap-1">
          <IconCoin className="h-3 w-3" /> Cost
        </div>
        <div data-testid="run-cost">{formatCostSubcents(run.costs.cost_subcents)}</div>
      </div>
    </div>
  );
}

function TokensRow({ run }: { run: RunDetail }) {
  return (
    <div className="flex items-center gap-4 text-xs text-muted-foreground" data-testid="run-tokens">
      <span>
        <span className="font-medium">In:</span> {formatTokens(run.costs.input_tokens)}
      </span>
      <span>
        <span className="font-medium">Out:</span> {formatTokens(run.costs.output_tokens)}
      </span>
      <span>
        <span className="font-medium">Cached:</span> {formatTokens(run.costs.cached_tokens)}
      </span>
    </div>
  );
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-red-200 dark:border-red-900 bg-red-50 dark:bg-red-950/30 p-2 text-sm">
      <IconAlertTriangle className="h-4 w-4 text-red-600 dark:text-red-400 mt-0.5 flex-shrink-0" />
      <pre
        className="text-xs text-red-700 dark:text-red-300 whitespace-pre-wrap break-words font-mono flex-1"
        data-testid="run-error-message"
      >
        {message}
      </pre>
    </div>
  );
}

type ActionBarProps = {
  runId?: string;
  sessionId?: string;
  isRunning: boolean;
  isFailed: boolean;
};

function ActionBar({ runId, sessionId, isRunning, isFailed }: ActionBarProps) {
  const handleRecover = async (action: "resume" | "fresh_start") => {
    const client = getWebSocketClient();
    if (!client || !runId || !sessionId) return;
    try {
      await client.request("session.recover", {
        task_id: runId,
        session_id: sessionId,
        action,
      });
    } catch {
      // No-op — UI will reflect any subsequent state via WS.
    }
  };

  const handleCancel = async () => {
    const client = getWebSocketClient();
    if (!client || !sessionId) return;
    try {
      await client.request("session.cancel", { session_id: sessionId });
    } catch {
      // No-op
    }
  };

  return (
    <div className="flex items-center gap-2">
      {isRunning && (
        <Button
          variant="outline"
          size="sm"
          onClick={handleCancel}
          className="cursor-pointer gap-1.5"
          data-testid="run-cancel-button"
        >
          <IconPlayerStop className="h-3.5 w-3.5" /> Cancel
        </Button>
      )}
      {isFailed && sessionId && (
        <>
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleRecover("resume")}
            className="cursor-pointer gap-1.5"
            data-testid="run-resume-button"
          >
            <IconRefresh className="h-3.5 w-3.5" /> Resume session
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleRecover("fresh_start")}
            className="cursor-pointer gap-1.5"
            data-testid="run-fresh-start-button"
          >
            <IconPlayerPlay className="h-3.5 w-3.5" /> Start fresh
          </Button>
        </>
      )}
    </div>
  );
}
