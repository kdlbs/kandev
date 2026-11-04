"use client";

import { useEffect, useMemo, useState } from "react";
import { IconCpu, IconSearch, IconTerminal2, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useTranslation } from "react-i18next";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import {
  filterWorkloadsByQuery,
  resolveInitialBackgroundWorkTab,
  selectActiveWorkloads,
  selectHistoryWorkloads,
  selectWorkloadsForTab,
  type BackgroundWorkTab,
} from "@/lib/background-work/workload-views";
import { cn } from "@/lib/utils";

const TOUCH_TARGET_CLASS = "min-h-11 min-w-11";
import { workloadKindLabelKey } from "./presentation";
import { WorkloadStateBadge } from "./workload-state-badge";

export type BackgroundWorkOverviewProps = {
  workloads: WorkloadRunObservation[];
  isLoading: boolean;
  onOpenWorkload: (workload: WorkloadRunObservation) => void;
  /** Session identity; a change resets the selected tab and search. */
  resetKey?: string | null;
  onClose?: () => void;
  closeLabel?: string;
  className?: string;
  testId?: string;
  showHeader?: boolean;
  /** Phone surfaces keep 44px targets even on a narrow fine pointer. */
  touchTargets?: boolean;
  /** One-shot tab navigation from the mobile detail Back fallback. */
  requestedTab?: { tab: BackgroundWorkTab; token: number } | null;
  /** The last read failed; do not present an unverified empty list. */
  loadError?: boolean;
  onRetry?: () => void;
};

function useOverviewSelection(
  workloads: WorkloadRunObservation[],
  isLoading: boolean,
  resetKey: string | null | undefined,
) {
  const [tab, setTab] = useState<BackgroundWorkTab | null>(null);
  const [search, setSearch] = useState("");

  useEffect(() => {
    setTab(null);
    setSearch("");
  }, [resetKey]);

  useEffect(() => {
    if (tab !== null || isLoading) return;
    setTab(resolveInitialBackgroundWorkTab(workloads));
  }, [isLoading, tab, workloads]);

  if (isLoading && tab === null) {
    return { tab: null, search, setSearch, setTab };
  }
  return {
    tab: tab ?? resolveInitialBackgroundWorkTab(workloads),
    search,
    setSearch,
    setTab,
  };
}

export function BackgroundWorkOverview({
  workloads,
  isLoading,
  onOpenWorkload,
  resetKey,
  onClose,
  closeLabel,
  className,
  testId = "background-work-overview",
  showHeader = true,
  touchTargets = false,
  requestedTab = null,
  loadError = false,
  onRetry,
}: BackgroundWorkOverviewProps) {
  const { tab, search, setSearch, setTab } = useOverviewSelection(
    workloads,
    isLoading || (loadError && workloads.length === 0),
    resetKey,
  );
  const activeCount = useMemo(() => selectActiveWorkloads(workloads).length, [workloads]);
  const historyCount = useMemo(() => selectHistoryWorkloads(workloads).length, [workloads]);

  useEffect(() => {
    if (requestedTab) setTab(requestedTab.tab);
  }, [requestedTab, setTab]);

  return (
    <div className={cn("flex h-full min-h-0 min-w-0 flex-col", className)} data-testid={testId}>
      {showHeader && (
        <OverviewTopBar onClose={onClose} closeLabel={closeLabel} touchTargets={touchTargets} />
      )}
      {showOverviewPlaceholder(loadError, workloads.length, tab) ? (
        <OverviewPlaceholder loadError={loadError && workloads.length === 0} onRetry={onRetry} />
      ) : (
        <Tabs
          value={tab ?? "active"}
          onValueChange={(value) => setTab(value === "history" ? "history" : "active")}
          className="flex min-h-0 min-w-0 flex-1 flex-col gap-0"
        >
          <OverviewTabsRow
            search={search}
            setSearch={setSearch}
            activeCount={activeCount}
            historyCount={historyCount}
            touchTargets={touchTargets}
          />
          <div className="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-2 pt-2 pb-[calc(0.5rem+env(safe-area-inset-bottom,0px))]">
            <TabsContent
              value="active"
              className="min-w-0 outline-none"
              data-testid="background-work-panel-active"
            >
              <OverviewPanelBody
                value="active"
                workloads={workloads}
                search={search}
                onOpenWorkload={onOpenWorkload}
                touchTargets={touchTargets}
              />
            </TabsContent>
            <TabsContent
              value="history"
              className="min-w-0 outline-none"
              data-testid="background-work-panel-history"
            >
              <OverviewPanelBody
                value="history"
                workloads={workloads}
                search={search}
                onOpenWorkload={onOpenWorkload}
                touchTargets={touchTargets}
              />
            </TabsContent>
          </div>
        </Tabs>
      )}
    </div>
  );
}

function showOverviewPlaceholder(
  loadError: boolean,
  workloadCount: number,
  tab: BackgroundWorkTab | null,
): boolean {
  return tab === null || (loadError && workloadCount === 0);
}

function OverviewPlaceholder({ loadError, onRetry }: { loadError: boolean; onRetry?: () => void }) {
  const { t } = useTranslation();
  if (!loadError) {
    return (
      <div
        className="flex min-h-0 flex-1 items-center justify-center px-3 py-8 text-center text-xs text-muted-foreground"
        role="status"
      >
        {t("task:loadingBackgroundWork")}
      </div>
    );
  }
  return (
    <div
      className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 px-3 py-8 text-center text-xs text-muted-foreground"
      role="alert"
    >
      <span>{t("task:backgroundWorkLoadFailed")}</span>
      {onRetry && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRetry}
          className="h-7 cursor-pointer text-xs"
          data-testid="background-work-retry"
        >
          {t("task:retry")}
        </Button>
      )}
    </div>
  );
}

function OverviewTopBar({
  onClose,
  closeLabel,
  touchTargets,
}: {
  onClose?: () => void;
  closeLabel?: string;
  touchTargets: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 items-center justify-between gap-2 border-b border-border px-3 py-2">
      <div className="flex min-w-0 items-center gap-2">
        <IconTerminal2 className="h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
        <span className="min-w-0 truncate font-semibold">{t("task:backgroundWorkOverview")}</span>
      </div>
      {onClose && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={onClose}
          aria-label={closeLabel ?? t("task:backgroundWorkClose")}
          className={cn("shrink-0 cursor-pointer", touchTargets ? TOUCH_TARGET_CLASS : "h-7 w-7")}
          data-testid="background-work-overview-close"
        >
          <IconX aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}

function OverviewTabsRow({
  search,
  setSearch,
  activeCount,
  historyCount,
  touchTargets,
}: {
  search: string;
  setSearch: (search: string) => void;
  activeCount: number;
  historyCount: number;
  touchTargets: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 border-b border-border px-2 py-1.5">
      <TabsList className={cn("h-auto min-w-0", touchTargets ? "min-h-12" : "min-h-8")}>
        <TabsTrigger
          value="active"
          className={cn("min-w-0 px-2", touchTargets ? TOUCH_TARGET_CLASS : "min-h-7")}
          data-testid="background-work-tab-active"
        >
          {t("task:backgroundTabActive", { count: activeCount })}
        </TabsTrigger>
        <TabsTrigger
          value="history"
          className={cn("min-w-0 px-2", touchTargets ? TOUCH_TARGET_CLASS : "min-h-7")}
          data-testid="background-work-tab-history"
        >
          {t("task:backgroundTabHistory", { count: historyCount })}
        </TabsTrigger>
      </TabsList>
      <div className="relative min-w-[8rem] flex-1 sm:max-w-48">
        <IconSearch
          className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t("task:searchJobs")}
          aria-label={t("task:searchJobs")}
          className={cn("min-w-0 pl-7 text-xs", touchTargets ? "h-11" : "h-7")}
          data-testid="background-work-search"
        />
      </div>
    </div>
  );
}

function OverviewPanelBody({
  value,
  workloads,
  search,
  onOpenWorkload,
  touchTargets,
}: {
  value: BackgroundWorkTab;
  workloads: WorkloadRunObservation[];
  search: string;
  onOpenWorkload: (workload: WorkloadRunObservation) => void;
  touchTargets: boolean;
}) {
  return (
    <OverviewBody
      workloads={workloads}
      tab={value}
      search={search}
      onOpenWorkload={onOpenWorkload}
      touchTargets={touchTargets}
    />
  );
}

function OverviewBody({
  workloads,
  tab,
  search,
  onOpenWorkload,
  touchTargets,
}: {
  workloads: WorkloadRunObservation[];
  tab: BackgroundWorkTab;
  search: string;
  onOpenWorkload: (workload: WorkloadRunObservation) => void;
  touchTargets: boolean;
}) {
  const { t } = useTranslation();
  const visible = filterWorkloadsByQuery(selectWorkloadsForTab(workloads, tab), search);
  if (visible.length === 0) {
    return (
      <div
        className="py-8 text-center text-xs text-muted-foreground"
        data-testid="background-work-empty"
      >
        {emptyStateCopy(t, tab, search.trim().length > 0)}
      </div>
    );
  }
  return (
    <div className="grid min-w-0 gap-2">
      {visible.map((workload) => (
        <WorkloadCard
          key={workload.work_id}
          workload={workload}
          onOpenWorkload={onOpenWorkload}
          touchTargets={touchTargets}
        />
      ))}
    </div>
  );
}

function emptyStateCopy(
  t: (key: string) => string,
  tab: BackgroundWorkTab,
  searching: boolean,
): string {
  if (searching) return t("task:noMatchingBackgroundWork");
  return tab === "active" ? t("task:noActiveBackgroundWork") : t("task:noBackgroundWorkHistory");
}

export function WorkloadCard({
  workload,
  onOpenWorkload,
  touchTargets,
}: {
  workload: WorkloadRunObservation;
  onOpenWorkload: (workload: WorkloadRunObservation) => void;
  touchTargets: boolean;
}) {
  return (
    <div
      data-testid={`background-workload-card-${workload.work_id}`}
      className="flex min-w-0 items-center justify-between gap-2 rounded-lg border border-border bg-card/60 p-2.5 shadow-sm transition-colors hover:bg-muted/40"
    >
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <WorkloadKindIcon kind={workload.kind} />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-1.5">
            <span
              className="min-w-0 flex-1 truncate font-medium text-sm text-foreground"
              title={workload.title}
              data-testid={`background-workload-title-${workload.work_id}`}
            >
              {workload.title}
            </span>
            <span className="shrink-0">
              <WorkloadStateBadge workload={workload} />
            </span>
          </div>
          <WorkloadKindLabel kind={workload.kind} />
        </div>
      </div>
      <OpenWorkloadButton
        workload={workload}
        onOpenWorkload={onOpenWorkload}
        touchTargets={touchTargets}
      />
    </div>
  );
}

function WorkloadKindIcon({ kind }: { kind: WorkloadRunObservation["kind"] }) {
  return (
    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
      {kind === "subagent" ? (
        <IconCpu className="h-4 w-4" aria-hidden="true" />
      ) : (
        <IconTerminal2 className="h-4 w-4" aria-hidden="true" />
      )}
    </div>
  );
}

function WorkloadKindLabel({ kind }: { kind: WorkloadRunObservation["kind"] }) {
  const { t } = useTranslation();
  return (
    <span className="block truncate text-[11px] text-muted-foreground">
      {t(workloadKindLabelKey(kind))}
    </span>
  );
}

function OpenWorkloadButton({
  workload,
  onOpenWorkload,
  touchTargets,
}: {
  workload: WorkloadRunObservation;
  onOpenWorkload: (workload: WorkloadRunObservation) => void;
  touchTargets: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className={cn("shrink-0 cursor-pointer text-xs", touchTargets ? TOUCH_TARGET_CLASS : "h-7")}
      onClick={() => onOpenWorkload(workload)}
      aria-label={`${t("task:openBackgroundWork")} ${workload.title}`}
      data-testid={`open-workload-${workload.work_id}`}
    >
      {t("task:openBackgroundWork")}
    </Button>
  );
}
