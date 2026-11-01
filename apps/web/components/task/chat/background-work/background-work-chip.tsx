"use client";

import {
  forwardRef,
  useId,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type ForwardedRef,
} from "react";
import { IconExternalLink, IconHistory, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useBackgroundWork } from "@/hooks/domains/session/use-background-work";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { cn } from "@/lib/utils";
import {
  workloadExitStatusKey,
  workloadKindLabelKey,
  workloadOutputUnavailableKey,
} from "./presentation";
import { WorkloadStateBadge } from "./workload-state-badge";

type WorkloadSummaryListProps = {
  workloads: WorkloadRunObservation[];
  runningCount: number;
  waitingCount: number;
  unknownCount: number;
  historyOnly: boolean;
  onOpenWorkload: (workId: string, title: string) => void;
  onOpenAll: () => void;
  drawer?: boolean;
};

function WorkloadSummaryCounts({
  runningCount,
  waitingCount,
  unknownCount,
}: {
  runningCount: number;
  waitingCount: number;
  unknownCount: number;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-1" data-testid="background-work-summary-counts">
      <Badge variant="default" className="text-[10px]">
        {t("task:backgroundRunningCount", { count: runningCount })}
      </Badge>
      <Badge variant="outline" className="text-[10px]">
        {t("task:backgroundWaitingCount", { count: waitingCount })}
      </Badge>
      <Badge variant="outline" className="text-[10px]">
        {t("task:backgroundUnknownCount", { count: unknownCount })}
      </Badge>
    </div>
  );
}

function WorkloadSummaryList({
  workloads,
  runningCount,
  waitingCount,
  unknownCount,
  historyOnly,
  onOpenWorkload,
  onOpenAll,
  drawer = false,
}: WorkloadSummaryListProps) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="background-work-summary-list"
      className={cn("flex flex-col gap-2", drawer && "min-h-0 overflow-y-auto p-4")}
      data-vaul-no-drag={drawer ? true : undefined}
    >
      <div className="flex items-center justify-between gap-2 border-b border-border pb-2">
        <span className="text-xs font-semibold text-foreground">
          {t(historyOnly ? "task:backgroundWorkHistoryTitle" : "task:backgroundWorkOverview")}
        </span>
        <WorkloadSummaryCounts
          runningCount={runningCount}
          waitingCount={waitingCount}
          unknownCount={unknownCount}
        />
      </div>
      <div className="flex max-h-56 flex-col gap-1.5 overflow-y-auto">
        {workloads.map((w) => (
          <div
            key={w.work_id}
            data-testid={`background-work-summary-item-${w.work_id}`}
            className="flex items-center justify-between gap-2 rounded border border-border/60 bg-muted/30 px-2 py-1.5 text-xs"
          >
            <div className="flex min-w-0 flex-1 flex-col">
              <span className="truncate font-medium text-foreground">{w.title}</span>
              <span className="text-[10px] text-muted-foreground">
                {t(workloadKindLabelKey(w.kind))}
              </span>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <WorkloadStateBadge workload={w} />
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-6 cursor-pointer px-2 text-[11px] [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
                onClick={() => onOpenWorkload(w.work_id, w.title)}
                data-testid={`background-work-open-button-${w.work_id}`}
              >
                {t("task:openBackgroundWork")}
                <IconExternalLink className="ml-1 h-3 w-3" />
              </Button>
            </div>
          </div>
        ))}
      </div>
      <div className="pt-1">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="w-full cursor-pointer justify-center text-xs [@media(pointer:coarse)]:min-h-11"
          onClick={onOpenAll}
          data-testid="background-work-view-all-button"
        >
          {t("task:viewAllBackgroundWork")}
        </Button>
      </div>
    </div>
  );
}

type BackgroundWorkTriggerProps = {
  detailsId: string;
  count: number;
  historyOnly: boolean;
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerRef: { current: HTMLButtonElement | null };
  usesDrawer: boolean;
} & Omit<ComponentPropsWithoutRef<"button">, "ref">;

const BackgroundWorkTrigger = forwardRef<HTMLButtonElement, BackgroundWorkTriggerProps>(
  function BackgroundWorkTrigger(
    { detailsId, count, historyOnly, open, setOpen, triggerRef, usesDrawer, ...buttonProps },
    forwardedRef: ForwardedRef<HTMLButtonElement>,
  ) {
    const { t } = useTranslation();
    const label = historyOnly
      ? t("task:backgroundJobHistory")
      : t("task:backgroundJobs", { count });
    return (
      <button
        {...buttonProps}
        type="button"
        ref={(node) => {
          triggerRef.current = node;
          if (typeof forwardedRef === "function") forwardedRef(node);
          else if (forwardedRef) forwardedRef.current = node;
        }}
        data-testid={historyOnly ? "background-work-history-chip" : "background-work-chip"}
        aria-label={label}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={detailsId}
        onClick={() => setOpen(!open)}
        className={cn(
          buttonProps.className,
          "inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-full border text-[11px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          historyOnly
            ? "border-border bg-muted/40 text-muted-foreground hover:bg-muted"
            : "border-primary/30 bg-primary/10 text-primary hover:bg-primary/20",
          usesDrawer ? "min-h-11 min-w-11 px-3" : "h-6 px-2.5",
        )}
      >
        {historyOnly ? (
          <IconHistory className="h-3 w-3" aria-hidden="true" />
        ) : (
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
          </span>
        )}
        <span>{label}</span>
      </button>
    );
  },
);

function WorkloadDetailDrawerHeader({
  workload,
  onBack,
}: {
  workload: WorkloadRunObservation;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  const exitKey = workloadExitStatusKey(workload);
  return (
    <DrawerHeader className="shrink-0 flex-col gap-1 border-b border-border pb-2 text-left">
      <div className="flex w-full items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onBack}
            className="h-7 cursor-pointer px-2 text-xs [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
            data-testid="background-work-drawer-back"
          >
            {t("task:back")}
          </Button>
          <DrawerTitle className="truncate text-xs font-semibold">{workload.title}</DrawerTitle>
        </div>
        <DrawerClose asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("task:backgroundWorkClose")}
            className="[@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
          >
            <IconX aria-hidden="true" />
          </Button>
        </DrawerClose>
      </div>
      <div className="flex flex-wrap items-center gap-2 pl-2">
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
        <span className="text-[10px] text-muted-foreground">
          {t(workloadKindLabelKey(workload.kind))}
        </span>
      </div>
    </DrawerHeader>
  );
}

function WorkloadDetailBody({ workload }: { workload: WorkloadRunObservation }) {
  const { t } = useTranslation();
  return (
    <div className="flex-1 min-h-0 overflow-y-auto bg-black/95 p-3 font-mono text-xs text-green-400">
      {workload.output_truncated && (
        <div className="mb-2 text-[10px] text-amber-400">[{t("task:earlierOutputTruncated")}]</div>
      )}
      <pre className="whitespace-pre-wrap break-all">
        {workload.output || t(workloadOutputUnavailableKey(workload))}
      </pre>
    </div>
  );
}

function BackgroundWorkDrawerBody({
  inspected,
  workloads,
  runningCount,
  waitingCount,
  unknownCount,
  historyOnly,
  onOpenWorkload,
  onOpenAll,
  onBack,
}: {
  inspected: WorkloadRunObservation | null;
  workloads: WorkloadRunObservation[];
  runningCount: number;
  waitingCount: number;
  unknownCount: number;
  historyOnly: boolean;
  onOpenWorkload: (workId: string, title: string) => void;
  onOpenAll: () => void;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  if (inspected) {
    return (
      <div
        className="flex h-full min-h-0 flex-1 flex-col overflow-hidden pb-[env(safe-area-inset-bottom)]"
        data-testid="background-work-drawer-detail"
        data-vaul-no-drag
      >
        <WorkloadDetailDrawerHeader workload={inspected} onBack={onBack} />
        <WorkloadDetailBody workload={inspected} />
      </div>
    );
  }

  return (
    <>
      <DrawerHeader className="shrink-0 flex-row items-center justify-between gap-3 text-left">
        <div className="min-w-0">
          <DrawerTitle>
            {t(historyOnly ? "task:backgroundWorkHistoryTitle" : "task:backgroundWorkDetailsTitle")}
          </DrawerTitle>
          <DrawerDescription>
            {t(
              historyOnly
                ? "task:backgroundWorkHistoryDescription"
                : "task:backgroundWorkDetailsDescription",
            )}
          </DrawerDescription>
        </div>
        <DrawerClose asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("task:backgroundWorkClose")}
            className="[@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
          >
            <IconX aria-hidden="true" />
          </Button>
        </DrawerClose>
      </DrawerHeader>
      <WorkloadSummaryList
        workloads={workloads}
        runningCount={runningCount}
        waitingCount={waitingCount}
        unknownCount={unknownCount}
        historyOnly={historyOnly}
        onOpenWorkload={onOpenWorkload}
        onOpenAll={onOpenAll}
        drawer
      />
    </>
  );
}

type BackgroundWorkChipProps = {
  sessionId: string | null;
  detailActive?: boolean;
};

export function BackgroundWorkChip(props: BackgroundWorkChipProps) {
  const { sessionId, detailActive = true } = props;
  const enabled = useFeature("agentBackgroundWork");
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const usesDrawer = !isFinePointer || isMobile;
  const [open, setOpen] = useState(false);
  const [selectedWorkId, setSelectedWorkId] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const detailsId = `background-work-details-${useId()}`;
  const { workloads, totalCount, totalJobCount, runningCount, waitingCount, unknownCount } =
    useBackgroundWork(sessionId, { detailActive });

  const historyOnly = totalCount === 0;

  const handleOpenWorkload = (workId: string, title: string) => {
    if (usesDrawer) {
      setSelectedWorkId(workId);
      return;
    }
    setOpen(false);
    useDockviewStore.getState().addBackgroundWorkPanel?.({
      sessionId: sessionId ?? undefined,
      workId,
      title,
      inCenter: true,
    });
  };

  const handleOpenAll = () => {
    if (usesDrawer) {
      setSelectedWorkId(null);
      return;
    }
    setOpen(false);
    useDockviewStore.getState().addBackgroundWorkPanel?.({
      sessionId: sessionId ?? undefined,
      inCenter: true,
    });
  };

  if (!enabled || totalJobCount === 0) return null;

  const inspected = selectedWorkId ? workloads.find((w) => w.work_id === selectedWorkId) : null;

  return (
    <BackgroundWorkSurface
      detailsId={detailsId}
      open={open}
      setOpen={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) setSelectedWorkId(null);
      }}
      usesDrawer={usesDrawer}
      triggerRef={triggerRef}
      totalCount={totalCount}
      historyOnly={historyOnly}
      workloads={workloads}
      runningCount={runningCount}
      waitingCount={waitingCount}
      unknownCount={unknownCount}
      inspected={inspected ?? null}
      onOpenWorkload={handleOpenWorkload}
      onOpenAll={handleOpenAll}
      onBack={() => setSelectedWorkId(null)}
    />
  );
}

function BackgroundWorkSurface({
  detailsId,
  open,
  setOpen,
  usesDrawer,
  triggerRef,
  totalCount,
  historyOnly,
  workloads,
  runningCount,
  waitingCount,
  unknownCount,
  inspected,
  onOpenWorkload,
  onOpenAll,
  onBack,
}: {
  detailsId: string;
  open: boolean;
  setOpen: (open: boolean) => void;
  usesDrawer: boolean;
  triggerRef: { current: HTMLButtonElement | null };
  totalCount: number;
  historyOnly: boolean;
  workloads: WorkloadRunObservation[];
  runningCount: number;
  waitingCount: number;
  unknownCount: number;
  inspected: WorkloadRunObservation | null;
  onOpenWorkload: (workId: string, title: string) => void;
  onOpenAll: () => void;
  onBack: () => void;
}) {
  const trigger = (
    <BackgroundWorkTrigger
      detailsId={detailsId}
      count={totalCount}
      historyOnly={historyOnly}
      open={open}
      setOpen={setOpen}
      triggerRef={triggerRef}
      usesDrawer={usesDrawer}
    />
  );

  if (usesDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          id={detailsId}
          data-testid="background-work-drawer-content"
          className={cn(
            inspected
              ? "max-h-[100dvh]"
              : "max-h-[min(80dvh,calc(100dvh-env(safe-area-inset-bottom)))]",
          )}
          // Vaul's bottom drawer caps itself at 80vh and offsets the sheet.
          // A focused job detail is a full-height surface with one scroll
          // owner, so the cap and offset are overridden for that state only.
          style={inspected ? { height: "100dvh", maxHeight: "100dvh", marginTop: 0 } : undefined}
        >
          <BackgroundWorkDrawerBody
            inspected={inspected}
            workloads={workloads}
            runningCount={runningCount}
            waitingCount={waitingCount}
            unknownCount={unknownCount}
            historyOnly={historyOnly}
            onOpenWorkload={onOpenWorkload}
            onOpenAll={onOpenAll}
            onBack={onBack}
          />
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        id={detailsId}
        data-testid="background-work-popover"
        side="top"
        align="start"
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        <WorkloadSummaryList
          workloads={workloads}
          runningCount={runningCount}
          waitingCount={waitingCount}
          unknownCount={unknownCount}
          historyOnly={historyOnly}
          onOpenWorkload={onOpenWorkload}
          onOpenAll={onOpenAll}
        />
      </PopoverContent>
    </Popover>
  );
}
