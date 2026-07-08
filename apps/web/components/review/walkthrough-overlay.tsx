"use client";

import { useEffect, useRef, useState } from "react";
import { IconRoute } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { getTaskWalkthrough } from "@/lib/api/domains/walkthrough-api";
import { WalkthroughFloatingWindow } from "@/components/diff/walkthrough-floating-window";
import { clearOpenWalkthroughTaskId, setOpenWalkthroughTaskId } from "@/lib/walkthrough-open-state";
import { cn } from "@kandev/ui/lib/utils";

type WalkthroughOverlayProps = {
  /** The task whose walkthrough launcher should be shown. */
  taskId: string | null;
  /** Unused — kept for call-site compatibility. */
  sessionId?: string | null;
  onSelectFile?: (path: string, repo?: string) => void | Promise<void>;
};

/**
 * Task-level launcher for an agent-authored walkthrough. It (1) backfills the
 * walkthrough into the store on mount — a live `task.walkthrough.created` event
 * can fire before the page's WS subscription exists — and (2) toggles the
 * floating step card, which opens each step's file (current state) and reveals
 * the anchored line. Works for changed and unchanged files alike (no review
 * surface required).
 */
export function WalkthroughOverlay({ taskId, onSelectFile }: WalkthroughOverlayProps) {
  const walkthrough = useAppStore((s) => (taskId ? s.walkthroughs.byTaskId[taskId] : null));
  const connectionStatus = useAppStore((s) => s.connection.status);
  const activeStep = useAppStore((s) =>
    taskId ? (s.walkthroughs.activeStepByTaskId[taskId] ?? 0) : 0,
  );
  const lastSeenUpdatedAt = useAppStore((s) =>
    taskId ? s.walkthroughs.lastSeenUpdatedAtByTaskId[taskId] : undefined,
  );
  const setWalkthrough = useAppStore((s) => s.setWalkthrough);
  const [open, setOpen] = useState(false);

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
        fetchedRef.current.add(taskId);
        if (wt) setWalkthrough(taskId, wt);
      })
      .catch(() => {})
      .finally(() => {
        inFlightRef.current.delete(taskId);
      });
    return () => {
      cancelled = true;
    };
  }, [taskId, walkthrough, connectionStatus, setWalkthrough]);

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
    setOpen(true);
  };
  const closeTour = () => {
    clearOpenWalkthroughTaskId(taskId);
    setOpen(false);
  };

  return (
    <>
      {open ? <WalkthroughFloatingWindow onClose={closeTour} onSelectFile={onSelectFile} /> : null}
      <button
        type="button"
        data-testid="walkthrough-launcher"
        aria-pressed={open}
        onClick={() => (open ? closeTour() : openTour())}
        className={cn(
          "fixed bottom-6 right-6 z-[60] flex cursor-pointer items-center gap-1.5 rounded-full",
          "border bg-card/95 px-3 py-1.5 text-xs font-medium text-foreground shadow-lg backdrop-blur-sm",
          "transition-colors hover:border-primary/45 hover:bg-muted/70",
          open ? "border-primary/50 bg-card ring-2 ring-primary/25" : "border-border/80",
        )}
      >
        <IconRoute className="size-4 text-primary" />
        Walkthrough
        {hasUnseen ? (
          <span
            aria-label="New walkthrough"
            className="size-1.5 rounded-full bg-primary"
            data-testid="walkthrough-unseen-dot"
          />
        ) : null}
        <span
          className={cn(
            "rounded-full px-1.5 py-0.5 text-[11px] font-medium",
            open ? "bg-primary/10 text-foreground" : "bg-muted/70 text-muted-foreground",
          )}
        >
          {activeStep + 1}/{walkthrough.steps.length}
        </span>
      </button>
    </>
  );
}
