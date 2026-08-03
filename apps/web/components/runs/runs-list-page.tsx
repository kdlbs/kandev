"use client";

import { useTranslation } from "react-i18next";
import { useCallback, useMemo } from "react";
import { IconBolt, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { PageTopbar } from "@/components/page-topbar";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { cn } from "@/lib/utils";
import type { WorkspaceAutomationRun } from "@/lib/types/automation";
import { buildAgenda, STATE_DOT_CLASS } from "./automation-rows";
import type { AutomationRow } from "./automation-rows";
import { RunFeedItem } from "./run-feed-item";
import { AUTOMATIONS_HREF, RUNS_FEED_HREF } from "./runs-view";
import { useAutomationSummaries } from "./use-automation-summaries";
import { useLiveRefresh } from "./use-live-refresh";
import { useWorkspaceAutomations } from "./use-workspace-automations";
import { useWorkspaceRuns } from "./use-workspace-runs";

const NEW_AUTOMATION_HREF = "/settings/automations";
const CENTERED_BLOCK = "flex flex-col items-center gap-3 py-16 text-center";
const SECTION_LABEL =
  "px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70";

/** How much of the feed this page shows before sending the reader to the lens. */
const INLINE_FEED_LIMIT = 12;

/**
 * The constraint is stated, not discovered. A schedule that quietly does
 * nothing overnight because the machine was asleep is the single most confusing
 * thing about this feature, and no per-row message can explain it after the
 * fact — the run that would have carried the explanation is the one that never
 * happened. It sits under the agenda because that is exactly what it qualifies.
 */
function SchedulerNote() {
  const { t } = useTranslation();
  return (
    <p className="px-1 pt-2 text-xs text-muted-foreground" data-testid="runs-scheduler-note">
      {t("automations:schedulerNote")}
    </p>
  );
}

function NoAutomationsYet() {
  const { t } = useTranslation();
  return (
    <div className={CENTERED_BLOCK} data-testid="runs-empty-state">
      <IconBolt className="h-8 w-8 text-muted-foreground/40" />
      <p className="text-sm font-medium">{t("automations:emptyListTitle")}</p>
      <p className="max-w-md text-sm text-muted-foreground">{t("automations:emptyListBody")}</p>
      <Button asChild size="sm" className="cursor-pointer">
        <Link href={NEW_AUTOMATION_HREF}>{t("automations:createAnAutomation")}</Link>
      </Button>
    </div>
  );
}

/**
 * One line of the agenda: when, and what. The name is secondary here — the
 * sidebar already lists automations by name, so this page leads with the time,
 * which is the fact only it can show.
 */
function AgendaRow({ row, onOpen }: { row: AutomationRow; onOpen: (id: string) => void }) {
  const { automation, state, next } = row;
  return (
    <button
      type="button"
      onClick={() => onOpen(automation.id)}
      data-testid={`agenda-row-${automation.id}`}
      className="flex w-full items-baseline gap-3 rounded-md px-3 py-2 text-left transition-colors cursor-pointer hover:bg-muted/40"
    >
      <span
        className={cn(
          "h-1.5 w-1.5 shrink-0 translate-y-[-1px] rounded-full",
          STATE_DOT_CLASS[state],
        )}
        aria-hidden="true"
      />
      <span
        className={cn(
          "w-44 shrink-0 text-sm tabular-nums",
          next.kind === "reason" ? "text-amber-600 dark:text-amber-500" : "text-foreground",
        )}
        data-testid="automation-next-run"
      >
        {next.text}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
        {automation.name}
      </span>
    </button>
  );
}

function Agenda({ rows, onOpen }: { rows: AutomationRow[]; onOpen: (id: string) => void }) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-1" data-testid="automation-agenda">
      <p className={SECTION_LABEL}>{t("automations:upNext")}</p>
      {rows.map((row) => (
        <AgendaRow key={row.automation.id} row={row} onOpen={onOpen} />
      ))}
      <SchedulerNote />
    </section>
  );
}

/**
 * What every automation has been saying, newest first. Inline rather than
 * behind a link: "did anything go wrong overnight" is the other half of why
 * someone opens this page, and a link would make them ask for it twice.
 */
function RecentRuns({
  runs,
  loading,
  onOpen,
}: {
  runs: WorkspaceAutomationRun[];
  loading: boolean;
  onOpen: (taskId: string) => void;
}) {
  const { t } = useTranslation();
  if (runs.length === 0) {
    if (loading) return null;
    return (
      <section className="flex flex-col gap-1" data-testid="recent-runs">
        <p className={SECTION_LABEL}>{t("automations:recentRuns")}</p>
        <p className="px-3 py-4 text-sm text-muted-foreground">
          {t("automations:nothingHasRunYet")}
        </p>
      </section>
    );
  }
  return (
    <section className="flex flex-col gap-1" data-testid="recent-runs">
      <div className="flex items-baseline justify-between gap-3">
        <p className={SECTION_LABEL}>{t("automations:recentRuns")}</p>
        <Link
          href={RUNS_FEED_HREF}
          data-testid="runs-feed-link"
          className="cursor-pointer text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          {t("automations:allRuns")}
        </Link>
      </div>
      <div className="flex flex-col divide-y divide-border/50">
        {runs.slice(0, INLINE_FEED_LIMIT).map((run) => (
          <RunFeedItem
            key={run.id}
            run={run}
            automationName={run.automation_name}
            onOpen={onOpen}
          />
        ))}
      </div>
    </section>
  );
}

export function RunsListPage({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  // The route hands down the boot payload's workspace, which is captured once
  // and never changes for the life of the SPA session. This page follows the
  // ACTIVE workspace, so prefer the live store value.
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const effectiveWorkspaceId = activeWorkspaceId ?? workspaceId;
  const automationsState = useWorkspaceAutomations(effectiveWorkspaceId);
  const summariesState = useAutomationSummaries(effectiveWorkspaceId);
  const runsState = useWorkspaceRuns(effectiveWorkspaceId);

  const rows = useMemo(
    () => buildAgenda(automationsState.automations, summariesState.summaries),
    [automationsState.automations, summariesState.summaries],
  );
  const loading = automationsState.loading || summariesState.loading || runsState.loading;
  const refresh = useCallback(() => {
    automationsState.refresh();
    summariesState.refresh();
    runsState.refresh();
  }, [automationsState, summariesState, runsState]);

  // Poll only while something is actually open — an idle workspace makes no
  // requests at all.
  useLiveRefresh(
    rows.some((row) => row.state === "running"),
    refresh,
  );

  const error = automationsState.error ?? summariesState.error ?? runsState.error;
  const empty = rows.length === 0 && !loading;

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <PageTopbar
        title={t("automations:automations")}
        icon={<IconBolt className="h-4 w-4" />}
        actions={
          <Button
            variant="ghost"
            size="icon-sm"
            className="cursor-pointer"
            onClick={refresh}
            disabled={loading}
            title={t("automations:refresh")}
            data-testid="runs-refresh"
          >
            <IconRefresh className={loading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
          </Button>
        }
      />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 px-4 py-5">
          {error && (
            <div className={CENTERED_BLOCK} data-testid="runs-error">
              <p className="text-sm text-destructive">{error}</p>
              <Button variant="outline" size="sm" className="cursor-pointer" onClick={refresh}>
                {t("automations:tryAgain")}
              </Button>
            </div>
          )}
          {!error && empty && <NoAutomationsYet />}
          {!error && !empty && (
            <>
              <Agenda
                rows={rows}
                onOpen={(automationId) => router.push(`${AUTOMATIONS_HREF}/${automationId}`)}
              />
              <RecentRuns
                runs={runsState.runs}
                loading={loading}
                onOpen={(taskId) => router.push(`/tasks/${taskId}`)}
              />
            </>
          )}
        </div>
      </div>
    </div>
  );
}
