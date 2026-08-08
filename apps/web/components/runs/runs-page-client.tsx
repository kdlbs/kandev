"use client";

import { useTranslation } from "react-i18next";
import { useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { IconHistory, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { PageTopbar } from "@/components/page-topbar";
import { useRouter } from "@/lib/routing/client-router";
import type { WorkspaceAutomationRun } from "@/lib/types/automation";
import { RunFeedItem } from "./run-feed-item";
import { ANY_AUTOMATION, RunFilters, isDefaultFilters } from "./run-filters";
import { ALL_STATUSES, type RunStatusFilter } from "./run-status";
import { AUTOMATIONS_HREF } from "./runs-view";
import { useWorkspaceRuns } from "./use-workspace-runs";

const NEW_AUTOMATION_HREF = "/settings/automations";

type RunsPageClientProps = {
  workspaceId?: string;
};

/** Distinct automations present in the feed, in first-seen (newest) order. */
function automationOptions(runs: WorkspaceAutomationRun[]) {
  const seen = new Map<string, string>();
  for (const run of runs) {
    if (!seen.has(run.automation_id)) seen.set(run.automation_id, run.automation_name);
  }
  return Array.from(seen, ([id, name]) => ({ id, name }));
}

function matchesFilters(
  run: WorkspaceAutomationRun,
  status: RunStatusFilter,
  automationId: string,
): boolean {
  if (status !== ALL_STATUSES && run.status !== status) return false;
  return automationId === ANY_AUTOMATION || run.automation_id === automationId;
}

function NoRunsYet() {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-col items-center gap-3 py-16 text-center"
      data-testid="runs-empty-state"
    >
      <IconHistory className="h-8 w-8 text-muted-foreground/40" />
      <p className="text-sm font-medium">{t("automations:emptyRunsTitle")}</p>
      <p className="max-w-md text-sm text-muted-foreground">{t("automations:emptyListBody")}</p>
      <Button asChild size="sm" className="cursor-pointer">
        <Link href={NEW_AUTOMATION_HREF}>{t("automations:createAnAutomation")}</Link>
      </Button>
    </div>
  );
}

function NoFilterMatches({ onClear }: { onClear: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-col items-center gap-3 py-16 text-center"
      data-testid="runs-filtered-empty-state"
    >
      <p className="text-sm text-muted-foreground">{t("automations:noRunsMatchFilters")}</p>
      <Button variant="outline" size="sm" className="cursor-pointer" onClick={onClear}>
        {t("automations:clearFilters")}
      </Button>
    </div>
  );
}

type RunFeedProps = {
  runs: WorkspaceAutomationRun[];
  allRuns: WorkspaceAutomationRun[];
  loading: boolean;
  error: string | null;
  onClearFilters: () => void;
  onOpen: (taskId: string) => void;
};

function RunFeed({ runs, allRuns, loading, error, onClearFilters, onOpen }: RunFeedProps) {
  const { t } = useTranslation();
  if (error) {
    return (
      <p className="py-16 text-center text-sm text-destructive" data-testid="runs-error">
        {error}
      </p>
    );
  }
  if (loading && allRuns.length === 0) {
    return (
      <p className="py-16 text-center text-sm text-muted-foreground">
        {t("automations:loadingRuns")}
      </p>
    );
  }
  if (allRuns.length === 0) return <NoRunsYet />;
  if (runs.length === 0) return <NoFilterMatches onClear={onClearFilters} />;
  return (
    <div className="flex flex-col divide-y divide-border/50" data-testid="runs-feed">
      {runs.map((run) => (
        <RunFeedItem key={run.id} run={run} automationName={run.automation_name} onOpen={onOpen} />
      ))}
    </div>
  );
}

export function RunsPageClient({ workspaceId }: RunsPageClientProps) {
  const { t } = useTranslation();
  const router = useRouter();
  // The route hands down the boot payload's workspace, which is captured once
  // and never changes for the life of the SPA session. /runs follows the
  // ACTIVE workspace, so prefer the live store value — otherwise landing here
  // after a workspace switch keeps querying whichever workspace was active at
  // boot.
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const effectiveWorkspaceId = activeWorkspaceId ?? workspaceId;
  const { runs, loading, error, refresh } = useWorkspaceRuns(effectiveWorkspaceId);
  const [status, setStatus] = useState<RunStatusFilter>(ALL_STATUSES);
  const [automationId, setAutomationId] = useState<string>(ANY_AUTOMATION);

  const automations = useMemo(() => automationOptions(runs), [runs]);
  // An automation id only means something inside the workspace that owns it.
  // After a workspace switch — or when an automation drops out of the feed —
  // the chip falls back to naming "Any automation" while the filter still
  // excludes everything, so the user gets an empty feed with no visible cause.
  // Degrading a selection nothing can name back to "any" keeps what is shown
  // and what is filtered in agreement.
  const effectiveAutomationId = useMemo(
    () =>
      automationId === ANY_AUTOMATION || automations.some((a) => a.id === automationId)
        ? automationId
        : ANY_AUTOMATION,
    [automationId, automations],
  );
  const visibleRuns = useMemo(
    () => runs.filter((run) => matchesFilters(run, status, effectiveAutomationId)),
    [runs, status, effectiveAutomationId],
  );

  const clearFilters = () => {
    setStatus(ALL_STATUSES);
    setAutomationId(ANY_AUTOMATION);
  };

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <PageTopbar
        title={t("automations:runsTitle")}
        icon={<IconHistory className="h-4 w-4" />}
        backHref={AUTOMATIONS_HREF}
        // Names the destination, not this page: back goes to the automations
        // list, and a crumb reading "Runs" pointed at a page titled
        // "Automations" left the reader guessing which one they were on.
        backLabel={t("automations:automations")}
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
        <div className="mx-auto w-full max-w-3xl px-4 py-4">
          <div className="mb-2 flex items-center justify-between gap-2">
            <RunFilters
              status={status}
              onStatusChange={setStatus}
              automationId={effectiveAutomationId}
              onAutomationChange={setAutomationId}
              automations={automations}
            />
            {!isDefaultFilters(status, effectiveAutomationId) && (
              <span className="text-xs text-muted-foreground">
                {visibleRuns.length} of {runs.length}
              </span>
            )}
          </div>
          <RunFeed
            runs={visibleRuns}
            allRuns={runs}
            loading={loading}
            error={error}
            onClearFilters={clearFilters}
            onOpen={(taskId) => router.push(`/tasks/${taskId}`)}
          />
        </div>
      </div>
    </div>
  );
}
