"use client";

import { useTranslation } from "react-i18next";
import { useCallback, useEffect, useState } from "react";
import { IconBolt, IconPlayerPlay, IconPlayerStop, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PageTopbar } from "@/components/page-topbar";
import { AutomationEditor } from "@/components/automations/automation-editor";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { cn } from "@/lib/utils";
import { stopAutomationRun } from "@/lib/api/domains/automation-api";
import type { Automation, AutomationRun } from "@/lib/types/automation";
import { nextFiring } from "./automation-rows";
import { RunsDrawer } from "./runs-drawer";
import { RunsRail } from "./runs-rail";
import { RunTranscript } from "./run-transcript";
import { isOpenRun } from "./run-status";
import { AUTOMATIONS_HREF, runHref, type AutomationDetailTab } from "./runs-view";
import { useAutomationActivity } from "./use-automation-activity";
import { useLiveRefresh } from "./use-live-refresh";
import { useRailWidth } from "./use-rail-width";
import { useManualTrigger } from "./use-manual-trigger";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { statusLabelKey } from "./run-status";

const MUTED_NOTE = "py-16 text-center text-sm text-muted-foreground";

/**
 * An automation belongs to one workspace. Switching workspaces with this page
 * open would otherwise leave the previous workspace's automation on screen
 * indefinitely, under a sidebar that says the user is somewhere else.
 */
function useWorkspaceGuard(automation: Automation | null, onLeave: () => void): boolean {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const foreign = Boolean(
    automation && activeWorkspaceId && automation.workspace_id !== activeWorkspaceId,
  );
  useEffect(() => {
    if (foreign) onLeave();
  }, [foreign, onLeave]);
  return foreign;
}

/**
 * The run the pane is showing. An explicit `?run=` wins; otherwise the newest,
 * because "what did it say last night" is the question this page exists for.
 * A requested run that is not in the window falls back rather than rendering an
 * empty pane that reads as a broken link.
 */
function selectRun(runs: AutomationRun[], requestedId: string | undefined): AutomationRun | null {
  const openable = runs.filter((run) => Boolean(run.session_id));
  if (openable.length === 0) return null;
  const requested = requestedId ? openable.find((run) => run.id === requestedId) : undefined;
  if (requested) return requested;
  return openable.reduce((newest, run) =>
    Date.parse(run.created_at) > Date.parse(newest.created_at) ? run : newest,
  );
}

/**
 * The page's own topbar rather than a hand-rolled header: PageTopbar is `h-10`,
 * which is what the sidebar header is, and the two sit side by side. A bespoke
 * header drifts out of alignment the moment either one changes.
 */
function DetailHeader({
  automation,
  openRuns,
  runNow,
  triggering,
  refresh,
  loading,
  disabled,
}: {
  automation: Automation | null;
  openRuns: number;
  runNow: () => void;
  triggering: boolean;
  refresh: () => void;
  loading: boolean;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const next = automation ? nextFiring(automation, openRuns) : null;
  return (
    <PageTopbar
      title={automation?.name ?? t("automations:automation")}
      icon={<IconBolt className="h-4 w-4" />}
      backHref={AUTOMATIONS_HREF}
      backLabel={t("automations:automations")}
      leftActions={
        next ? (
          <span
            className={cn(
              // Hidden on a phone: the bar already carries a back label, the
              // automation's name and two actions, and this note has no width
              // budget left — it overran the title. The same information is a
              // tap away in the runs drawer, which states the schedule.
              "hidden truncate text-xs sm:inline",
              next.kind === "reason"
                ? "text-amber-600 dark:text-amber-500"
                : "text-muted-foreground",
            )}
            data-testid="automation-next-run"
          >
            {next.text}
          </span>
        ) : undefined
      }
      actions={
        <>
          {/* Waiting until tomorrow to find out whether a schedule works is the
              reason this sits on the reading surface and not only in settings. */}
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer text-xs"
            onClick={runNow}
            disabled={disabled}
            data-testid="automation-run-now"
          >
            <IconPlayerPlay className="h-3.5 w-3.5" />
            {triggering ? t("automations:starting") : t("automations:runNow")}
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            className="cursor-pointer"
            onClick={refresh}
            disabled={loading}
            title={t("automations:refresh")}
            data-testid="automation-refresh"
          >
            <IconRefresh className={loading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
          </Button>
        </>
      }
    />
  );
}

function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-col items-center gap-3 py-16 text-center"
      data-testid="automation-error"
    >
      <p className="text-sm text-destructive">{message}</p>
      <Button variant="outline" size="sm" className="cursor-pointer" onClick={onRetry}>
        {t("automations:tryAgain")}
      </Button>
    </div>
  );
}

/**
 * Configuration, reached from the rail's Details link rather than a tab beside
 * the transcript. A tab pair claims the two are done about equally; an
 * automation is configured once and read continuously.
 */
function ConfigureView({ automation }: { automation: Automation }) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto w-full max-w-3xl px-4 py-4">
        <SettingsSaveProvider>
          <AutomationEditor workspaceId={automation.workspace_id} automationId={automation.id} />
        </SettingsSaveProvider>
      </div>
    </div>
  );
}

/**
 * The conversation, and nothing above it.
 *
 * The standing instruction used to be pinned here as a header block. It is the
 * same text on every run and it is long, so it pushed what the agent actually
 * said down the page on every visit. It now lives behind the run-detail button
 * in the rail (or the drawer, on a phone) — asked for once, rather than paid
 * for every time.
 */
function ActivityView({
  automationId,
  selected,
  hasRuns,
  loading,
  refresh,
}: {
  automationId: string;
  selected: AutomationRun | null;
  hasRuns: boolean;
  loading: boolean;
  refresh: () => void;
}) {
  const { t } = useTranslation();
  const [stopping, setStopping] = useState(false);
  const [stopError, setStopError] = useState<string | null>(null);

  useEffect(() => {
    setStopping(false);
    setStopError(null);
  }, [selected?.id]);

  const handleStop = async () => {
    if (!selected || !isOpenRun(selected.status) || stopping) return;
    setStopping(true);
    setStopError(null);
    try {
      await stopAutomationRun(automationId, selected.id);
      refresh();
    } catch (error) {
      setStopError(error instanceof Error ? error.message : t("common:requestFailed"));
    } finally {
      setStopping(false);
    }
  };

  if (!selected) {
    if (loading && !hasRuns)
      return <p className={MUTED_NOTE}>{t("automations:loadingActivity")}</p>;
    return (
      <p className={MUTED_NOTE} data-testid="automation-activity-empty">
        {hasRuns ? t("automations:noConversationForRun") : t("automations:hasNotRunYet")}
      </p>
    );
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="automation-activity">
      <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 px-4 py-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium" data-testid="run-display-title">
            {selected.display_title || t("automations:run")}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {t(statusLabelKey(selected.status))}
          </p>
        </div>
        {isOpenRun(selected.status) && (
          <Button
            variant="destructive"
            size="sm"
            className="min-h-11 shrink-0 cursor-pointer"
            onClick={() => void handleStop()}
            disabled={stopping}
            data-testid="automation-stop-run"
          >
            <IconPlayerStop className="h-3.5 w-3.5" />
            {stopping ? t("automations:stopping") : t("automations:stopCurrentRun")}
          </Button>
        )}
      </div>
      {stopError && (
        <p className="shrink-0 px-4 py-2 text-sm text-destructive" role="alert">
          {t("automations:failedToStopRun", { error: stopError })}
        </p>
      )}
      {/* Keyed on the exact session turn so selecting two runs in one shared
          session remounts the conversation for the requested firing. */}
      <RunTranscript
        key={`${selected.session_id}:${selected.turn_id ?? ""}:${selected.id}`}
        sessionId={selected.session_id ?? ""}
        taskId={selected.task_id || null}
        turnId={selected.turn_id}
      />
    </div>
  );
}

/**
 * The conversation and its run switcher.
 *
 * Split out so the page component stays inside the complexity budget once the
 * responsive branch is added, and so the two compositions sit side by side
 * where a reader can compare them.
 */
function DetailBody({
  automation,
  tab,
  runs,
  selected,
  openRuns,
  loading,
  refresh,
  isMobile,
  rail,
  onSelectRun,
}: {
  automation: Automation;
  tab: AutomationDetailTab;
  runs: AutomationRun[];
  selected: AutomationRun | null;
  openRuns: number;
  loading: boolean;
  refresh: () => void;
  isMobile: boolean;
  rail: ReturnType<typeof useRailWidth>;
  onSelectRun: (run: AutomationRun) => void;
}) {
  const switcher = isMobile ? (
    <RunsDrawer
      automation={automation}
      runs={runs}
      selectedRunId={selected?.id ?? null}
      onSelect={onSelectRun}
      openRuns={openRuns}
    />
  ) : (
    <RunsRail
      automation={automation}
      runs={runs}
      selectedRunId={selected?.id ?? null}
      onSelect={onSelectRun}
      openRuns={openRuns}
      width={rail.width}
      resizing={rail.resizing}
      onResizeStart={rail.onResizeStart}
    />
  );

  return (
    <div
      className={cn(
        "flex min-h-0 flex-1 overflow-hidden",
        // On a phone the switcher sits above the conversation rather than
        // beside it; a side-by-side split leaves neither usable.
        isMobile && "flex-col",
      )}
    >
      {isMobile && switcher}
      {/* min-w-0 or the transcript's wide code blocks push the rail off screen
          instead of scrolling inside their own container. */}
      <main className="flex min-h-0 min-w-0 flex-1 flex-col">
        {tab === "configure" ? (
          <ConfigureView automation={automation} />
        ) : (
          <ActivityView
            automationId={automation.id}
            selected={selected}
            hasRuns={runs.length > 0}
            loading={loading}
            refresh={refresh}
          />
        )}
      </main>
      {!isMobile && switcher}
    </div>
  );
}

export function AutomationDetailPage({
  automationId,
  tab,
  runId,
}: {
  automationId: string;
  tab: AutomationDetailTab;
  runId?: string;
}) {
  const router = useRouter();
  const { automation, runs, openRuns, loading, error, refresh } =
    useAutomationActivity(automationId);
  const { runNow, triggering, settling } = useManualTrigger(automationId, refresh);
  const leaveForList = useCallback(() => router.push(AUTOMATIONS_HREF), [router]);
  const foreign = useWorkspaceGuard(automation, leaveForList);
  // `settling` as well as `openRuns`: the run row is written after the fire
  // returns, so gating only on what the page can already see leaves a window
  // where a just-fired run appears and finishes without the page ever asking
  // again — and every note derived from that snapshot, including the amber one
  // explaining why the automation will not fire next, goes stale with it.
  // A visible open row is a second live signal: the summary deliberately
  // excludes some task/session states, but the rail still truthfully says
  // Running until the run query returns a terminal status.
  const hasVisibleOpenRun = runs.some((run) => isOpenRun(run.status));
  useLiveRefresh(!foreign && (openRuns > 0 || hasVisibleOpenRun || settling), refresh);
  const rail = useRailWidth();
  const { isMobile } = useResponsiveBreakpoint();

  const selected = selectRun(runs, runId);
  const show = !error && !foreign && automation;

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <DetailHeader
        automation={foreign ? null : automation}
        openRuns={openRuns}
        runNow={() => void runNow()}
        triggering={triggering}
        refresh={refresh}
        loading={loading}
        disabled={triggering || !automation || foreign}
      />
      {error && <LoadError message={error} onRetry={refresh} />}
      {show && (
        <DetailBody
          automation={automation}
          tab={tab}
          runs={runs}
          selected={selected}
          openRuns={openRuns}
          loading={loading}
          refresh={refresh}
          isMobile={isMobile}
          rail={rail}
          onSelectRun={(run) => router.push(runHref(automationId, run.id))}
        />
      )}
    </div>
  );
}
