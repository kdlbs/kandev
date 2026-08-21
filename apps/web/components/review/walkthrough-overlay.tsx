"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { IconRoute, IconX } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { deleteTaskWalkthrough, getTaskWalkthrough } from "@/lib/api/domains/walkthrough-api";
import { WalkthroughFloatingWindow } from "@/components/diff/walkthrough-floating-window";
import { clearOpenWalkthroughTaskId, setOpenWalkthroughTaskId } from "@/lib/walkthrough-open-state";
import { cn } from "@kandev/ui/lib/utils";
import type { TaskWalkthrough } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type WalkthroughOverlayProps = {
  /** The task whose walkthrough launcher should be shown. */
  taskId: string | null;
  /** Unused — kept for call-site compatibility. */
  sessionId?: string | null;
  onSelectFile?: (path: string, repo?: string) => void | Promise<void>;
};

type WalkthroughLauncherProps = {
  activeStep: number;
  confirmDiscardOpen: boolean;
  discardAnchorRef: RefObject<HTMLButtonElement | null>;
  discardDisabled: boolean;
  hasUnseen: boolean;
  isFinePointer: boolean;
  isOpen: boolean;
  onDiscardClick: () => void;
  onDiscardCancel: () => void;
  onDiscardClose: () => void;
  onDiscardConfirm: () => void | Promise<void>;
  onToggle: () => void;
  stepCount: number;
};

function WalkthroughLauncher({
  activeStep,
  confirmDiscardOpen,
  discardAnchorRef,
  discardDisabled,
  hasUnseen,
  isFinePointer,
  isOpen,
  onDiscardClick,
  onDiscardCancel,
  onDiscardClose,
  onDiscardConfirm,
  onToggle,
  stepCount,
}: WalkthroughLauncherProps) {
  const { t } = useTranslation();
  const discardLabel = t("review:discardWalkthrough");
  const discardVisibility = isFinePointer
    ? "opacity-0 pointer-events-none scale-90 group-hover:pointer-events-auto group-hover:scale-100 group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:scale-100 group-focus-within:opacity-100"
    : "";
  return (
    <div className="group fixed bottom-[calc(1.5rem+var(--app-status-bar-height))] right-6 z-[41]">
      <button
        type="button"
        data-testid="walkthrough-launcher"
        aria-pressed={isOpen}
        onClick={onToggle}
        className={cn(
          "flex cursor-pointer items-center gap-1.5 rounded-full",
          "border bg-card/95 px-3 py-1.5 pr-4 text-xs font-medium text-foreground shadow-lg backdrop-blur-sm",
          "transition-colors hover:border-primary/45 hover:bg-muted/70",
          isOpen ? "border-primary/50 bg-card ring-2 ring-primary/25" : "border-border/80",
        )}
      >
        <IconRoute className="size-4 text-primary" />
        {t("review:walkthrough")}
        {hasUnseen ? (
          <span
            aria-label={t("review:newWalkthrough")}
            className="size-1.5 rounded-full bg-primary"
            data-testid="walkthrough-unseen-dot"
          />
        ) : null}
        <span
          className={cn(
            "rounded-full px-1.5 py-0.5 text-[11px] font-medium",
            isOpen ? "bg-primary/10 text-foreground" : "bg-muted/70 text-muted-foreground",
          )}
        >
          {activeStep + 1}/{stepCount}
        </span>
      </button>
      {isFinePointer || !confirmDiscardOpen ? (
        <button
          ref={discardAnchorRef}
          type="button"
          aria-label={discardLabel}
          title={discardLabel}
          data-testid="walkthrough-discard"
          disabled={discardDisabled}
          onClick={onDiscardClick}
          className={cn(
            "absolute -right-3 -top-3 flex cursor-pointer items-center justify-center rounded-full",
            isFinePointer ? "h-7 min-h-7 w-7 min-w-7" : "h-11 min-h-11 w-11 min-w-11",
            "border border-border bg-card text-muted-foreground shadow-md transition-all",
            "hover:border-destructive/40 hover:bg-destructive/10 hover:text-destructive",
            "disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-45",
            discardVisibility,
          )}
        >
          <IconX className="size-4" />
        </button>
      ) : (
        <div className="absolute bottom-full right-0 z-[1] mb-2 rounded-lg border border-border bg-card/95 p-1 shadow-lg backdrop-blur-sm">
          <InlineConfirmActions
            density="touch"
            testId="walkthrough-discard-confirmation"
            ariaLabel={t("review:discardWalkthroughTitle")}
            cancelLabel={t("common:cancel")}
            confirmLabel={discardLabel}
            confirmAriaLabel={discardLabel}
            onCancel={onDiscardCancel}
            onClose={onDiscardClose}
            onConfirm={onDiscardConfirm}
          />
        </div>
      )}
    </div>
  );
}

function useWalkthroughBackfill(params: {
  connectionStatus: string;
  setWalkthrough: (taskId: string, walkthrough: TaskWalkthrough | null) => void;
  taskId: string | null;
  walkthrough: TaskWalkthrough | null | undefined;
}) {
  const { connectionStatus, setWalkthrough, taskId, walkthrough } = params;
  const fetchedRef = useRef<Set<string>>(new Set());
  const inFlightRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (
      !taskId ||
      walkthrough ||
      connectionStatus !== "connected" ||
      fetchedRef.current.has(taskId) ||
      inFlightRef.current.has(taskId)
    ) {
      return;
    }
    let cancelled = false;
    inFlightRef.current.add(taskId);
    getTaskWalkthrough(taskId)
      .then((wt) => {
        if (cancelled) return;
        if (wt) {
          fetchedRef.current.add(taskId);
          setWalkthrough(taskId, wt);
        }
      })
      .catch(() => {})
      .finally(() => {
        inFlightRef.current.delete(taskId);
      });
    return () => {
      cancelled = true;
    };
  }, [connectionStatus, setWalkthrough, taskId, walkthrough]);
}

/**
 * Task-level launcher for an agent-authored walkthrough. It (1) backfills the
 * walkthrough into the store on mount — a live `task.walkthrough.created` event
 * can fire before the page's WS subscription exists — and (2) toggles the
 * floating step card, which opens each step's file (current state) and reveals
 * the anchored line. Works for changed and unchanged files alike (no review
 * surface required).
 */
export function WalkthroughOverlay({ taskId, onSelectFile }: WalkthroughOverlayProps) {
  const { t } = useTranslation();
  const discardLabel = t("review:discardWalkthrough");
  const { isFinePointer } = useResponsiveBreakpoint();
  const walkthrough = useAppStore((s) => (taskId ? s.walkthroughs.byTaskId[taskId] : null));
  const connectionStatus = useAppStore((s) => s.connection.status);
  const activeStep = useAppStore((s) =>
    taskId ? (s.walkthroughs.activeStepByTaskId[taskId] ?? 0) : 0,
  );
  const lastSeenUpdatedAt = useAppStore((s) =>
    taskId ? s.walkthroughs.lastSeenUpdatedAtByTaskId[taskId] : undefined,
  );
  const setWalkthrough = useAppStore((s) => s.setWalkthrough);
  const [openTaskId, setOpenTaskId] = useState<string | null>(null);
  const [discardTargetTaskId, setDiscardTargetTaskId] = useState<string | null>(null);
  const [discarding, setDiscarding] = useState(false);
  const discardAnchorRef = useRef<HTMLButtonElement>(null);
  const { toast } = useToast();
  const open = taskId !== null && openTaskId === taskId;
  const confirmDiscardOpen = taskId !== null && discardTargetTaskId === taskId;

  useWalkthroughBackfill({ connectionStatus, setWalkthrough, taskId, walkthrough });

  useEffect(() => {
    if (!taskId || !open) return;
    setOpenWalkthroughTaskId(taskId);
    return () => clearOpenWalkthroughTaskId(taskId);
  }, [open, taskId]);

  if (!taskId || !walkthrough) return null;
  const hasUnseen = walkthrough.updated_at !== lastSeenUpdatedAt;

  // Refresh to the latest persisted walkthrough when opening the card — covers
  // the case where the agent re-emitted a walkthrough and the live WS update
  // was missed (e.g. the tab was idle), so it shows without a page reload.
  const openTour = () => {
    setOpenWalkthroughTaskId(taskId);
    getTaskWalkthrough(taskId)
      .then((wt) => {
        if (wt) setWalkthrough(taskId, wt);
      })
      .catch(() => {});
    setOpenTaskId(taskId);
  };
  const closeTour = () => {
    clearOpenWalkthroughTaskId(taskId);
    setOpenTaskId(null);
  };
  const discardWalkthrough = async (targetTaskId: string | null) => {
    if (!targetTaskId || discarding) return;
    setDiscarding(true);
    try {
      await deleteTaskWalkthrough(targetTaskId);
      clearOpenWalkthroughTaskId(targetTaskId);
      setOpenTaskId((currentTaskId) => (currentTaskId === targetTaskId ? null : currentTaskId));
      setWalkthrough(targetTaskId, null);
      setDiscardTargetTaskId(null);
      toast({ title: t("review:walkthroughDiscarded"), variant: "success" });
    } catch (error) {
      console.error("Failed to discard walkthrough:", error);
      toast({ title: t("review:failedToDiscardWalkthrough"), variant: "error" });
    } finally {
      setDiscarding(false);
    }
  };

  return (
    <>
      {open ? <WalkthroughFloatingWindow onClose={closeTour} onSelectFile={onSelectFile} /> : null}
      <WalkthroughLauncher
        activeStep={activeStep}
        confirmDiscardOpen={confirmDiscardOpen}
        discardAnchorRef={discardAnchorRef}
        discardDisabled={discarding}
        hasUnseen={hasUnseen}
        isFinePointer={isFinePointer}
        isOpen={open}
        onDiscardClick={() => setDiscardTargetTaskId(taskId)}
        onDiscardCancel={() => {
          setDiscardTargetTaskId(null);
          requestAnimationFrame(() => discardAnchorRef.current?.focus());
        }}
        onDiscardClose={() => setDiscardTargetTaskId(null)}
        onDiscardConfirm={() => discardWalkthrough(discardTargetTaskId)}
        onToggle={() => (open ? closeTour() : openTour())}
        stepCount={walkthrough.steps.length}
      />
      {isFinePointer ? (
        <ActionConfirmPopover
          open={confirmDiscardOpen}
          anchorRef={discardAnchorRef}
          title={t("review:discardWalkthroughTitle")}
          description={t("review:discardWalkthroughDescription")}
          cancelLabel={t("common:cancel")}
          confirmLabel={discardLabel}
          confirmTestId="walkthrough-discard-confirm"
          testId="walkthrough-discard-confirmation"
          onOpenChange={(nextOpen) => setDiscardTargetTaskId(nextOpen ? taskId : null)}
          onConfirm={() => discardWalkthrough(discardTargetTaskId)}
        />
      ) : null}
    </>
  );
}
