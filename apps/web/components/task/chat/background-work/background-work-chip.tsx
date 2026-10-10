"use client";

import {
  forwardRef,
  useId,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type ForwardedRef,
} from "react";
import { IconExternalLink, IconX } from "@tabler/icons-react";
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

type WorkloadSummaryListProps = {
  workloads: WorkloadRunObservation[];
  onOpenWorkload: (workId: string, title: string) => void;
  onOpenAll: () => void;
  drawer?: boolean;
};

function WorkloadSummaryList({
  workloads,
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
          {t("task:backgroundWorkOverview")}
        </span>
        <Badge variant="outline" className="text-[10px]">
          {t("task:backgroundJobs", { count: workloads.length })}
        </Badge>
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
              <span className="text-[10px] text-muted-foreground capitalize">{w.kind}</span>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <Badge
                variant={w.state === "running" ? "default" : "outline"}
                className={cn(
                  "text-[10px] capitalize",
                  w.state === "running" && "bg-primary text-primary-foreground",
                )}
              >
                {w.state}
              </Badge>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-6 cursor-pointer px-2 text-[11px]"
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
          className="w-full cursor-pointer justify-center text-xs"
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
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerRef: { current: HTMLButtonElement | null };
  usesDrawer: boolean;
} & Omit<ComponentPropsWithoutRef<"button">, "ref">;

const BackgroundWorkTrigger = forwardRef<HTMLButtonElement, BackgroundWorkTriggerProps>(
  function BackgroundWorkTrigger(
    { detailsId, count, open, setOpen, triggerRef, usesDrawer, ...buttonProps },
    forwardedRef: ForwardedRef<HTMLButtonElement>,
  ) {
    const { t } = useTranslation();
    return (
      <button
        {...buttonProps}
        type="button"
        ref={(node) => {
          triggerRef.current = node;
          if (typeof forwardedRef === "function") forwardedRef(node);
          else if (forwardedRef) forwardedRef.current = node;
        }}
        data-testid="background-work-chip"
        aria-label={t("task:backgroundJobs", { count })}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={detailsId}
        onClick={() => setOpen(!open)}
        className={cn(
          buttonProps.className,
          "inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-full border border-primary/30 bg-primary/10 text-[11px] font-medium text-primary transition-colors hover:bg-primary/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          usesDrawer ? "min-h-11 min-w-11 px-3" : "h-6 px-2.5",
        )}
      >
        <span className="relative flex h-2 w-2">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
        </span>
        <span>{t("task:backgroundJobs", { count })}</span>
      </button>
    );
  },
);

function BackgroundWorkDrawerBody({
  inspected,
  workloads,
  onOpenWorkload,
  onOpenAll,
  onBack,
  outputEndRef,
}: {
  inspected: WorkloadRunObservation | null;
  workloads: WorkloadRunObservation[];
  onOpenWorkload: (workId: string, title: string) => void;
  onOpenAll: () => void;
  onBack: () => void;
  outputEndRef: React.RefObject<HTMLDivElement | null>;
}) {
  const { t } = useTranslation();
  if (inspected) {
    return (
      <div className="flex h-full max-h-[75dvh] flex-col overflow-hidden">
        <DrawerHeader className="shrink-0 flex-row items-center justify-between gap-3 border-b border-border text-left pb-2">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onBack}
              className="h-7 cursor-pointer px-2 text-xs"
              data-testid="background-work-drawer-back"
            >
              {t("task:back")}
            </Button>
            <DrawerTitle className="truncate text-xs font-semibold">{inspected.title}</DrawerTitle>
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
        <div className="flex-1 min-h-0 overflow-y-auto bg-black/95 p-3 font-mono text-xs text-green-400">
          <pre className="whitespace-pre-wrap break-all">
            {inspected.output || t("task:liveOutputUnavailable")}
          </pre>
          <div ref={outputEndRef} />
        </div>
      </div>
    );
  }

  return (
    <>
      <DrawerHeader className="shrink-0 flex-row items-center justify-between gap-3 text-left">
        <div className="min-w-0">
          <DrawerTitle>{t("task:backgroundWorkDetailsTitle")}</DrawerTitle>
          <DrawerDescription>{t("task:backgroundWorkDetailsDescription")}</DrawerDescription>
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
        onOpenWorkload={onOpenWorkload}
        onOpenAll={onOpenAll}
        drawer
      />
    </>
  );
}

export function BackgroundWorkChip({ sessionId }: { sessionId: string | null }) {
  const enabled = useFeature("agentBackgroundWork");
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const usesDrawer = !isFinePointer || isMobile;
  const [open, setOpen] = useState(false);
  const [selectedWorkId, setSelectedWorkId] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const outputEndRef = useRef<HTMLDivElement>(null);
  const detailsId = `background-work-details-${useId()}`;
  const { workloads, totalCount } = useBackgroundWork(sessionId);

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

  if (!enabled || totalCount === 0) return null;

  const trigger = (
    <BackgroundWorkTrigger
      detailsId={detailsId}
      count={totalCount}
      open={open}
      setOpen={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) setSelectedWorkId(null);
      }}
      triggerRef={triggerRef}
      usesDrawer={usesDrawer}
    />
  );

  const inspected = selectedWorkId ? workloads.find((w) => w.work_id === selectedWorkId) : null;

  if (usesDrawer) {
    return (
      <Drawer
        open={open}
        onOpenChange={(isOpen) => {
          setOpen(isOpen);
          if (!isOpen) setSelectedWorkId(null);
        }}
      >
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          id={detailsId}
          data-testid="background-work-drawer-content"
          className="max-h-[min(80dvh,calc(100dvh-env(safe-area-inset-bottom)))]"
        >
          <BackgroundWorkDrawerBody
            inspected={inspected ?? null}
            workloads={workloads}
            onOpenWorkload={handleOpenWorkload}
            onOpenAll={handleOpenAll}
            onBack={() => setSelectedWorkId(null)}
            outputEndRef={outputEndRef}
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
          onOpenWorkload={handleOpenWorkload}
          onOpenAll={handleOpenAll}
        />
      </PopoverContent>
    </Popover>
  );
}
