"use client";

import { useEffect, useRef, useState, type ComponentPropsWithoutRef } from "react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useBackgroundWork } from "@/hooks/domains/session/use-background-work";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { cn } from "@/lib/utils";
import { BackgroundWorkMobileSurface } from "./background-work-mobile-surface";
import { RunningWorkSummary } from "./background-work-summary";

type BackgroundWorkChipProps = {
  sessionId: string | null;
  detailActive?: boolean;
  /** Stable composer focus target used when the chip disappears on dismissal. */
  onRequestComposerFocus?: () => void;
};

/**
 * Composer entry for background inspection. The chip itself, its count, and
 * its summary contain running work only; history stays in the full overview.
 */
export function BackgroundWorkChip(props: BackgroundWorkChipProps) {
  const { sessionId, detailActive = true, onRequestComposerFocus } = props;
  const enabled = useFeature("agentBackgroundWork");
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const usesDrawer = !isFinePointer || isMobile;
  const [open, setOpen] = useState(false);
  const [phoneInspectionActive, setPhoneInspectionActive] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const {
    workloads,
    isLoading,
    isInitialLoading,
    loadError,
    runningWorkloads,
    fetchWorkloads,
    executeAction,
  } = useBackgroundWork(sessionId, {
    detailActive,
  });
  const runningCount = runningWorkloads.length;
  const showChip = runningCount > 0;
  const keepPhoneSurface = usesDrawer && open && phoneInspectionActive;

  useEffect(() => {
    if (usesDrawer || !open || showChip) return;
    setOpen(false);
    onRequestComposerFocus?.();
  }, [onRequestComposerFocus, open, showChip, usesDrawer]);

  useEffect(() => {
    if (!usesDrawer || !open || phoneInspectionActive || runningCount > 0) return;
    setOpen(false);
    onRequestComposerFocus?.();
  }, [onRequestComposerFocus, open, phoneInspectionActive, runningCount, usesDrawer]);

  if (!enabled || (!showChip && !keepPhoneSurface)) return null;

  const close = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) setPhoneInspectionActive(false);
  };

  const handleCloseAutoFocus = (event: Event) => {
    if (triggerRef.current) return;
    event.preventDefault();
    onRequestComposerFocus?.();
  };

  const openDesktop = (workId?: string, title?: string) => {
    setOpen(false);
    useDockviewStore.getState().addBackgroundWorkPanel?.({
      sessionId: sessionId ?? undefined,
      workId,
      title,
      inCenter: true,
    });
  };

  if (usesDrawer) {
    return (
      <BackgroundWorkDrawer
        open={open}
        setOpen={close}
        showChip={showChip}
        runningCount={runningCount}
        workloads={workloads}
        runningWorkloads={runningWorkloads}
        isLoading={isLoading || isInitialLoading}
        loadError={loadError}
        onRetry={fetchWorkloads}
        onAction={executeAction}
        sessionId={sessionId}
        triggerRef={triggerRef}
        inspectionActive={phoneInspectionActive}
        onInspectionChange={setPhoneInspectionActive}
        onCloseAutoFocus={handleCloseAutoFocus}
      />
    );
  }

  return (
    <Popover open={open} onOpenChange={close}>
      <PopoverTrigger asChild>
        <BackgroundWorkTrigger
          ref={triggerRef}
          count={runningCount}
          open={open}
          usesDrawer={false}
        />
      </PopoverTrigger>
      <PopoverContent
        data-testid="background-work-popover"
        side="top"
        align="start"
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
        onCloseAutoFocus={handleCloseAutoFocus}
      >
        <RunningWorkSummary
          workloads={runningWorkloads}
          onOpenWorkload={openDesktop}
          onOpenAll={() => openDesktop()}
        />
      </PopoverContent>
    </Popover>
  );
}

function BackgroundWorkDrawer({
  open,
  setOpen,
  showChip,
  runningCount,
  workloads,
  runningWorkloads,
  isLoading,
  loadError,
  onRetry,
  onAction,
  sessionId,
  triggerRef,
  inspectionActive,
  onInspectionChange,
  onCloseAutoFocus,
}: {
  open: boolean;
  setOpen: (open: boolean) => void;
  showChip: boolean;
  runningCount: number;
  workloads: WorkloadRunObservation[];
  runningWorkloads: WorkloadRunObservation[];
  isLoading: boolean;
  loadError: boolean;
  onRetry: () => Promise<void>;
  onAction: (req: { action: "stop" | "interrupt"; work_id?: string }) => Promise<unknown>;
  sessionId: string | null;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  inspectionActive: boolean;
  onInspectionChange: (active: boolean) => void;
  onCloseAutoFocus: (event: Event) => void;
}) {
  const { t } = useTranslation();
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {showChip && (
        <DrawerTrigger asChild>
          <BackgroundWorkTrigger ref={triggerRef} count={runningCount} open={open} usesDrawer />
        </DrawerTrigger>
      )}
      <DrawerContent
        data-testid="background-work-drawer-content"
        className={cn(
          inspectionActive
            ? "max-h-[100dvh]"
            : "max-h-[min(80dvh,calc(100dvh-env(safe-area-inset-bottom)))]",
        )}
        onCloseAutoFocus={onCloseAutoFocus}
        style={
          inspectionActive ? { height: "100dvh", maxHeight: "100dvh", marginTop: 0 } : undefined
        }
      >
        <DrawerHeader className="sr-only">
          <DrawerTitle>{t("task:backgroundWorkOverview")}</DrawerTitle>
          <DrawerDescription>{t("task:backgroundWorkDetailsDescription")}</DrawerDescription>
        </DrawerHeader>
        <BackgroundWorkMobileSurface
          sessionId={sessionId}
          workloads={workloads}
          isLoading={isLoading}
          loadError={loadError}
          onRetry={onRetry}
          onAction={onAction}
          runningWorkloads={runningWorkloads}
          initialView="summary"
          onClose={() => setOpen(false)}
          onInspectionChange={onInspectionChange}
        />
      </DrawerContent>
    </Drawer>
  );
}

type BackgroundWorkTriggerProps = {
  ref: React.RefObject<HTMLButtonElement | null>;
  count: number;
  open: boolean;
  usesDrawer: boolean;
} & Omit<
  ComponentPropsWithoutRef<"button">,
  "ref" | "type" | "children" | "aria-label" | "aria-haspopup" | "aria-expanded"
>;

function BackgroundWorkTrigger({
  ref,
  count,
  open,
  usesDrawer,
  ...buttonProps
}: BackgroundWorkTriggerProps) {
  const { t } = useTranslation();
  const label = t("task:backgroundJobs", { count });
  return (
    <button
      {...buttonProps}
      type="button"
      ref={ref}
      data-testid="background-work-chip"
      aria-label={label}
      aria-haspopup="dialog"
      aria-expanded={open}
      className={cn(
        "inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-full border border-primary/30 bg-primary/10 text-[11px] font-medium text-primary transition-colors hover:bg-primary/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        usesDrawer ? "min-h-11 min-w-11 px-3" : "h-6 px-2.5",
      )}
    >
      <span className="relative flex h-2 w-2">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
      </span>
      <span>{label}</span>
    </button>
  );
}
