"use client";

import { memo } from "react";
import Link from "@/components/routing/app-link";
import { IconExternalLink, IconLoader2 } from "@tabler/icons-react";
import { Card } from "@kandev/ui/card";
import { CompositorSpin } from "@kandev/ui/compositor-spin";
import type { AgentSummary, SessionSummary } from "@/lib/api/domains/office-api";
import { AgentAvatar as RoleAwareAgentAvatar } from "./agent-avatar";
import { timeAgo } from "@/lib/utils/time";
import { useNow } from "@/hooks/use-now";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

type Props = { summary: AgentSummary };

/**
 * Persistent per-agent card on the office dashboard.
 *
 * Three visual states drive layout:
 *   - "live"      → pulsing dot, "Live now" subtitle, current task pill.
 *   - "finished"  → muted dot, "Finished {ago}" subtitle, last task pill.
 *   - "never_run" → muted dot, "Never run" subtitle, no pill, no rows.
 *
 * The card always renders at the same width regardless of state — see
 * spec acceptance #1: N agents → N cards, no shrinking on idle.
 */
function AgentCardInner({ summary }: Props) {
  const { t } = useTranslation();
  const isLive = summary.status === "live";
  const isErrored = isAutoPaused(summary) || summary.last_run_status === "failed";
  const subtitle = pickSubtitle(t, summary);
  const pill = pickActiveTask(summary);
  const recent = summary.recent_sessions ?? [];

  return (
    <Card data-testid={`agent-card-${summary.agent_id}`} className="p-4 flex flex-col gap-2">
      <CardHeader summary={summary} isLive={isLive} isErrored={isErrored} />
      <p data-testid="agent-card-subtitle" className="text-sm text-muted-foreground">
        {subtitle}
      </p>
      {pill ? <TaskPill pill={pill} isLive={isLive} /> : null}
      {recent.length > 0 ? (
        <div className="mt-2 border-t pt-2 space-y-1.5 max-h-32 overflow-auto">
          {recent.slice(0, 5).map((s) => (
            <SessionRow key={s.session_id} session={s} agentName={summary.agent_name} />
          ))}
        </div>
      ) : null}
    </Card>
  );
}

/**
 * Skip re-renders that would produce identical DOM. Each WS event on
 * the dashboard channel triggers a full agent-summaries refetch which
 * swaps `summary` for a freshly-deserialized object — even when
 * nothing visible changed. Without this gate, every event repaints
 * every card (and React DevTools highlight outlines flash on the
 * right edge of the recent-session row).
 */
function summariesEqualForRender(a: AgentSummary, b: AgentSummary): boolean {
  if (a === b) return true;
  if (a.agent_id !== b.agent_id) return false;
  if (a.agent_name !== b.agent_name) return false;
  if (a.agent_role !== b.agent_role) return false;
  if (a.status !== b.status) return false;
  if (a.last_run_status !== b.last_run_status) return false;
  if (a.pause_reason !== b.pause_reason) return false;
  if (a.consecutive_failures !== b.consecutive_failures) return false;
  if (!sessionEqual(a.live_session, b.live_session)) return false;
  if (!sessionEqual(a.last_session, b.last_session)) return false;
  return recentSessionsEqual(a.recent_sessions, b.recent_sessions);
}

function sessionEqual(a?: SessionSummary | null, b?: SessionSummary | null): boolean {
  if (a === b) return true;
  if (!a || !b) return false;
  return (
    a.session_id === b.session_id &&
    a.task_id === b.task_id &&
    a.task_identifier === b.task_identifier &&
    a.task_title === b.task_title &&
    a.state === b.state &&
    a.started_at === b.started_at &&
    a.completed_at === b.completed_at &&
    a.duration_seconds === b.duration_seconds &&
    a.command_count === b.command_count
  );
}

function recentSessionsEqual(a?: SessionSummary[], b?: SessionSummary[]): boolean {
  const aa = a ?? [];
  const bb = b ?? [];
  if (aa.length !== bb.length) return false;
  for (let i = 0; i < aa.length; i++) {
    if (!sessionEqual(aa[i], bb[i])) return false;
  }
  return true;
}

export const AgentCard = memo(AgentCardInner, (prev, next) =>
  summariesEqualForRender(prev.summary, next.summary),
);

function CardHeader({
  summary,
  isLive,
  isErrored,
}: {
  summary: AgentSummary;
  isLive: boolean;
  isErrored: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 min-w-0">
      <StatusDot isLive={isLive} isErrored={isErrored} />
      <RoleAwareAgentAvatar role={summary.agent_role} name={summary.agent_name} size="sm" />
      <span className="font-medium truncate flex-1">{summary.agent_name}</span>
      <Link
        href={`/office/agents/${summary.agent_id}`}
        aria-label={t("office:openAgent", { name: summary.agent_name })}
        className="cursor-pointer text-muted-foreground hover:text-foreground"
      >
        <IconExternalLink className="h-3.5 w-3.5" />
      </Link>
    </div>
  );
}

function StatusDot({ isLive, isErrored }: { isLive: boolean; isErrored: boolean }) {
  if (isLive) {
    return (
      <span data-testid="agent-card-live-dot" className="relative flex h-2 w-2 shrink-0">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
      </span>
    );
  }
  if (isErrored) {
    return (
      <span
        data-testid="agent-card-error-dot"
        className="inline-block h-2 w-2 rounded-full shrink-0 bg-red-500"
      />
    );
  }
  return <span className="inline-block h-2 w-2 rounded-full shrink-0 bg-muted-foreground/40" />;
}

type ActivePill = { taskId: string; identifier: string; title: string };

function TaskPill({ pill, isLive }: { pill: ActivePill; isLive: boolean }) {
  const { t } = useTranslation();
  return (
    <Link
      href={`/office/tasks/${pill.taskId}`}
      data-testid="agent-card-task-pill"
      className="flex items-center gap-2 rounded-md border bg-muted/30 px-2 py-1 text-sm hover:bg-muted/50 transition-colors cursor-pointer truncate"
    >
      {isLive ? (
        <CompositorSpin
          data-testid="agent-card-task-pill-spinner"
          className="h-3.5 w-3.5 shrink-0 text-primary"
        >
          <IconLoader2 aria-label={t("office:beingWorkedOn")} className="size-full" />
        </CompositorSpin>
      ) : null}
      {pill.identifier ? (
        <span className="font-mono text-xs text-muted-foreground shrink-0">{pill.identifier}</span>
      ) : null}
      <span className="truncate">{pill.title || pill.taskId}</span>
    </Link>
  );
}

/**
 * The whole row is one key per variant rather than a sentence assembled from a
 * verb, a duration and an optional command clause. The verb used to be a value
 * (`isLive ? "working" : "worked"`) dropped into a fixed English word order,
 * which no language can reorder or inflect; the command clause used a bare
 * plural noun. Four keys, two of them pluralized on `count`, keep the English
 * byte-identical and leave every language free.
 */
function sessionSummaryLine(
  t: TFunction,
  agent: string,
  isLive: boolean,
  seconds: number,
  commandCount: number,
): string {
  if (commandCount > 0) {
    const key = isLive
      ? "office:agentWorkingForSecondsRanCommands"
      : "office:agentWorkedForSecondsRanCommands";
    return t(key, { agent, seconds, count: commandCount });
  }
  return t(isLive ? "office:agentWorkingForSeconds" : "office:agentWorkedForSeconds", {
    agent,
    seconds,
  });
}

function SessionRow({ session, agentName }: { session: SessionSummary; agentName: string }) {
  const { t } = useTranslation();
  const isLive = session.state === "RUNNING";
  // 1s tick when live (duration counter), 30s otherwise (timeAgo label).
  const now = useNow(isLive ? 1000 : 30_000);
  // Backend-computed duration is the source of truth for IDLE / COMPLETED
  // sessions — `completed_at` may be unset on office IDLE rows, so a
  // wall-clock fallback would grow forever. For RUNNING sessions we DO
  // want the wall clock so the displayed duration ticks every second
  // without waiting on a WS refetch.
  const durationS = isLive
    ? Math.max(0, Math.floor((now - new Date(session.started_at).getTime()) / 1000))
    : session.duration_seconds;
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span className="truncate">
        {sessionSummaryLine(t, agentName, isLive, durationS, session.command_count)}
      </span>
      <span className="flex-1" />
      <span className="shrink-0">{timeAgo(session.started_at)}</span>
    </div>
  );
}

/**
 * `{{ago}}` carries `timeAgo`'s output, a locale-formatted relative timestamp
 * ("5m ago") rather than a translated label — the same shape as a formatted
 * date, so interpolating it is safe. The em-dash placeholder is a symbol and
 * stays a literal.
 */
function pickSubtitle(t: TFunction, summary: AgentSummary): string {
  if (isAutoPaused(summary)) {
    // `count` + `_one`/`_other`, never a hand-written "s": the plural rule
    // belongs in the catalog, not at this call site.
    return t("office:pausedConsecutiveFailures", { count: summary.consecutive_failures ?? 0 });
  }
  if (summary.status === "live") return t("office:liveNow");
  if (summary.status === "never_run") return t("office:neverRun");
  const last = summary.last_session;
  if (!last) return "-";
  const ts = last.completed_at ?? last.started_at;
  if (summary.last_run_status === "failed") {
    return t("office:lastRunFailedAgo", { ago: timeAgo(ts) });
  }
  return t("office:finishedAgo", { ago: timeAgo(ts) });
}

function isAutoPaused(summary: AgentSummary): boolean {
  return (summary.pause_reason ?? "").startsWith("Auto-paused:");
}

function pickActiveTask(summary: AgentSummary): ActivePill | null {
  const ref = summary.live_session ?? summary.last_session;
  if (!ref) return null;
  return { taskId: ref.task_id, identifier: ref.task_identifier, title: ref.task_title };
}
