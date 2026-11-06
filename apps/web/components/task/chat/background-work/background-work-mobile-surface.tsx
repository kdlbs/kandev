"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { IconPlayerPause, IconPlayerStop, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useBackgroundWork } from "@/hooks/domains/session/use-background-work";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { canRunWorkloadAction } from "@/lib/background-work/capabilities";
import { isActiveWorkload } from "@/lib/background-work/workload-views";
import { cn } from "@/lib/utils";
import { BackgroundWorkOverview } from "./background-work-overview";
import { RunningWorkSummary } from "./background-work-summary";
import {
  workloadExitStatusKey,
  workloadKindLabelKey,
  workloadOutputUnavailableKey,
} from "./presentation";
import { WorkloadStateBadge } from "./workload-state-badge";

type MobileMode = "summary" | "overview" | "detail";

function focusWorkloadRow(workId: string | null) {
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      const escapedId =
        typeof CSS !== "undefined" && typeof CSS.escape === "function"
          ? CSS.escape(String(workId))
          : String(workId).replace(/["\\]/g, "\\$&");
      const selector = `[data-testid="background-work-open-button-${escapedId}"], [data-testid="open-workload-${escapedId}"]`;
      const target = document.querySelector<HTMLElement>(selector);
      const fallback = document.querySelector<HTMLElement>(
        '[data-testid^="open-workload-"], [data-testid^="background-work-open-button-"], [data-testid="background-work-chip"]',
      );
      (target ?? fallback)?.focus();
    });
  });
}

type BackgroundWorkMobileSurfaceProps = {
  sessionId: string | null;
  workloads: WorkloadRunObservation[];
  runningWorkloads: WorkloadRunObservation[];
  isLoading: boolean;
  initialView: "summary" | "overview";
  onClose: () => void;
  closeOnSummary?: boolean;
  loadError?: boolean;
  onRetry?: () => void;
  onAction?: (req: { action: "stop" | "interrupt"; work_id?: string }) => Promise<unknown>;
  /** True when the user left the inset summary for the overview or a detail. */
  onInspectionChange?: (inspectionActive: boolean) => void;
};

/**
 * Full-height phone work surface. The running chip opens its summary inset;
 * View all and the independent Panels entry open the overview directly. The
 * overview stays mounted behind a detail so Back restores tab, search, and
 * scroll position.
 */
function useMobileSurfaceNavigation({
  initialView,
  workloads,
  onClose,
  onInspectionChange,
}: {
  initialView: "summary" | "overview";
  workloads: WorkloadRunObservation[];
  onClose: () => void;
  onInspectionChange?: (inspectionActive: boolean) => void;
}) {
  const [mode, setMode] = useState<MobileMode>(initialView);
  const [selectedWorkId, setSelectedWorkId] = useState<string | null>(null);
  const [detailOrigin, setDetailOrigin] = useState<Exclude<MobileMode, "detail">>(initialView);
  const [requestedTab, setRequestedTab] = useState<{
    tab: "active" | "history";
    token: number;
  } | null>(null);
  const tabRequestTokenRef = useRef(0);
  const inspected = useMemo(
    () => (selectedWorkId ? (workloads.find((w) => w.work_id === selectedWorkId) ?? null) : null),
    [selectedWorkId, workloads],
  );

  useEffect(() => {
    if (mode === "detail" && !inspected) setMode("overview");
  }, [inspected, mode]);

  useEffect(() => {
    onInspectionChange?.(mode !== "summary");
  }, [mode, onInspectionChange]);

  const openDetail = (workload: WorkloadRunObservation) => {
    setDetailOrigin(mode === "summary" ? "summary" : "overview");
    setSelectedWorkId(workload.work_id);
    setMode("detail");
  };

  const backFromDetail = () => {
    const workId = selectedWorkId;
    setSelectedWorkId(null);
    const inspectedIsRunning = inspected?.state === "running";
    if (detailOrigin === "summary" && inspectedIsRunning) {
      setMode("summary");
    } else {
      if (detailOrigin === "summary") {
        const targetTab = inspected && isActiveWorkload(inspected) ? "active" : "history";
        tabRequestTokenRef.current += 1;
        setRequestedTab({ tab: targetTab, token: tabRequestTokenRef.current });
      }
      setMode("overview");
    }
    focusWorkloadRow(workId);
  };

  const openOverview = () => {
    setSelectedWorkId(null);
    setMode("overview");
  };

  const close = () => {
    setSelectedWorkId(null);
    setMode(initialView);
    onClose();
  };

  return { mode, inspected, requestedTab, openDetail, backFromDetail, openOverview, close };
}

export function BackgroundWorkMobileSurface({
  sessionId,
  workloads,
  runningWorkloads,
  isLoading,
  initialView,
  onClose,
  closeOnSummary = true,
  onInspectionChange,
  loadError = false,
  onRetry,
  onAction,
}: BackgroundWorkMobileSurfaceProps) {
  const { t } = useTranslation();
  const { mode, inspected, requestedTab, openDetail, backFromDetail, openOverview, close } =
    useMobileSurfaceNavigation({
      initialView,
      workloads,
      onClose,
      onInspectionChange,
    });

  return (
    <div
      className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-background"
      data-testid="background-work-mobile-surface"
    >
      {mode === "summary" && (
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <RunningWorkSummary
            workloads={runningWorkloads}
            drawer
            onOpenWorkload={(workId) => {
              const workload = runningWorkloads.find((row) => row.work_id === workId);
              if (workload) openDetail(workload);
            }}
            onOpenAll={openOverview}
            onClose={closeOnSummary ? close : undefined}
          />
        </div>
      )}
      {mode !== "summary" && (
        <div
          className={cn("min-h-0 min-w-0 flex-1", mode === "detail" && "hidden")}
          aria-hidden={mode === "detail"}
        >
          <BackgroundWorkOverview
            workloads={workloads}
            isLoading={isLoading}
            loadError={loadError}
            onRetry={onRetry}
            resetKey={sessionId}
            onOpenWorkload={openDetail}
            onClose={close}
            closeLabel={t("task:backgroundWorkClose")}
            testId="background-work-mobile-overview"
            touchTargets
            requestedTab={requestedTab}
          />
        </div>
      )}
      {mode === "detail" && inspected && (
        <MobileWorkloadDetail
          workload={inspected}
          onBack={backFromDetail}
          onClose={close}
          onAction={onAction}
        />
      )}
    </div>
  );
}

function MobileWorkloadActions({
  workload,
  onAction,
}: {
  workload: WorkloadRunObservation;
  onAction?: (req: { action: "stop" | "interrupt"; work_id?: string }) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const canStop = Boolean(onAction) && canRunWorkloadAction(workload, "stop");
  const canInterrupt = Boolean(onAction) && canRunWorkloadAction(workload, "interrupt");
  return (
    <>
      {canInterrupt && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => void onAction?.({ work_id: workload.work_id, action: "interrupt" })}
          className="min-h-11 shrink-0 cursor-pointer px-2 text-[11px]"
          data-testid="interrupt-workload-button"
        >
          <IconPlayerPause className="mr-1 h-3 w-3" aria-hidden="true" />
          {t("task:interruptBackgroundWork")}
        </Button>
      )}
      {canStop && (
        <Button
          type="button"
          variant="destructive"
          size="sm"
          onClick={() => void onAction?.({ work_id: workload.work_id, action: "stop" })}
          className="min-h-11 shrink-0 cursor-pointer px-2 text-[11px]"
          data-testid="stop-workload-button"
        >
          <IconPlayerStop className="mr-1 h-3 w-3" aria-hidden="true" />
          {t("task:stopBackgroundWork")}
        </Button>
      )}
    </>
  );
}

function MobileWorkloadDetail({
  workload,
  onBack,
  onClose,
  onAction,
}: {
  workload: WorkloadRunObservation;
  onBack: () => void;
  onClose: () => void;
  onAction?: (req: { action: "stop" | "interrupt"; work_id?: string }) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const exitKey = workloadExitStatusKey(workload);
  return (
    <div
      className="flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden pb-[env(safe-area-inset-bottom)]"
      data-testid="background-work-drawer-detail"
    >
      <div className="shrink-0 border-b border-border px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onBack}
            className="min-h-11 min-w-11 shrink-0 cursor-pointer px-2 text-xs"
            data-testid="background-work-drawer-back"
          >
            {t("task:back")}
          </Button>
          <span className="min-w-0 flex-1 truncate text-xs font-semibold">
            {isActiveWorkload(workload)
              ? t("task:backgroundWorkDetailsTitle")
              : t("task:backgroundWorkHistoryTitle")}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onClose}
            aria-label={t("task:backgroundWorkClose")}
            className="min-h-11 min-w-11 shrink-0 cursor-pointer"
            data-testid="background-work-drawer-close"
          >
            <IconX aria-hidden="true" />
          </Button>
        </div>
        <div className="mt-1 flex min-w-0 flex-wrap items-center gap-2">
          <WorkloadStateBadge workload={workload} />
          {exitKey && (
            <Badge
              variant="outline"
              className="text-[10px] font-mono"
              data-testid="background-work-drawer-exit"
            >
              {t(exitKey, { code: workload.exit_code })}
            </Badge>
          )}
          <span className="min-w-0 truncate text-[10px] text-muted-foreground">
            {t(workloadKindLabelKey(workload.kind))}
          </span>
          <MobileWorkloadActions workload={workload} onAction={onAction} />
        </div>
      </div>
      <div
        className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden"
        data-testid="background-work-detail-scroll"
      >
        <div className="border-b border-border bg-background p-3">
          <p
            className="whitespace-pre-wrap break-words font-mono text-[11px] text-foreground [overflow-wrap:anywhere]"
            data-testid="background-work-detail-command"
          >
            {workload.title}
          </p>
        </div>
        <div className="bg-black/95 p-3 font-mono text-xs text-green-400">
          {workload.output_truncated && (
            <div className="mb-2 text-[10px] text-amber-400">
              [{t("task:earlierOutputTruncated")}]
            </div>
          )}
          <pre className="whitespace-pre-wrap break-all">
            {workload.output || t(workloadOutputUnavailableKey(workload))}
          </pre>
        </div>
      </div>
    </div>
  );
}

/** Independent phone entry rendered by the mobile panel area (Panels menu). */
export function BackgroundWorkMobilePanel({
  sessionId,
  onClose,
}: {
  sessionId: string | null;
  onClose: () => void;
}) {
  const {
    workloads,
    runningWorkloads,
    isLoading,
    isInitialLoading,
    loadError,
    fetchWorkloads,
    executeAction,
  } = useBackgroundWork(sessionId);
  return (
    <BackgroundWorkMobileSurface
      sessionId={sessionId}
      workloads={workloads}
      runningWorkloads={runningWorkloads}
      isLoading={isLoading || isInitialLoading}
      initialView="overview"
      onClose={onClose}
      closeOnSummary={false}
      loadError={loadError}
      onRetry={fetchWorkloads}
      onAction={executeAction}
    />
  );
}
