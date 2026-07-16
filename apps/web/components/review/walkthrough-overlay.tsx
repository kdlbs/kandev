"use client";

import { useEffect, useRef, useState, type MouseEvent } from "react";
import { IconRoute, IconX } from "@tabler/icons-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { deleteTaskWalkthrough, getTaskWalkthrough } from "@/lib/api/domains/walkthrough-api";
import { WalkthroughFloatingWindow } from "@/components/diff/walkthrough-floating-window";
import { clearOpenWalkthroughTaskId, setOpenWalkthroughTaskId } from "@/lib/walkthrough-open-state";
import { cn } from "@kandev/ui/lib/utils";
import type { TaskWalkthrough } from "@/lib/types/http";

type WalkthroughOverlayProps = {
  /** The task whose walkthrough launcher should be shown. */
  taskId: string | null;
  /** Unused — kept for call-site compatibility. */
  sessionId?: string | null;
  onSelectFile?: (path: string, repo?: string) => void | Promise<void>;
};

type WalkthroughLauncherProps = {
  activeStep: number;
  hasUnseen: boolean;
  isOpen: boolean;
  onDiscardClick: () => void;
  onToggle: () => void;
  stepCount: number;
};

function WalkthroughLauncher({
  activeStep,
  hasUnseen,
  isOpen,
  onDiscardClick,
  onToggle,
  stepCount,
}: WalkthroughLauncherProps) {
  return (
    <div className="group fixed bottom-6 right-6 z-[41]">
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
            isOpen ? "bg-primary/10 text-foreground" : "bg-muted/70 text-muted-foreground",
          )}
        >
          {activeStep + 1}/{stepCount}
        </span>
      </button>
      <button
        type="button"
        aria-label="Discard walkthrough"
        title="Discard walkthrough"
        data-testid="walkthrough-discard"
        onClick={onDiscardClick}
        className={cn(
          "absolute -right-3 -top-3 flex size-9 cursor-pointer items-center justify-center rounded-full",
          "border border-border bg-card text-muted-foreground shadow-md transition-all",
          "hover:border-destructive/40 hover:bg-destructive/10 hover:text-destructive",
          "sm:size-7 sm:opacity-0 sm:pointer-events-none sm:scale-90",
          "sm:group-hover:pointer-events-auto sm:group-hover:scale-100 sm:group-hover:opacity-100",
          "sm:group-focus-within:pointer-events-auto sm:group-focus-within:scale-100 sm:group-focus-within:opacity-100",
        )}
      >
        <IconX className="size-4" />
      </button>
    </div>
  );
}

type DiscardWalkthroughDialogProps = {
  discarding: boolean;
  onConfirm: (event: MouseEvent<HTMLButtonElement>) => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
};

function DiscardWalkthroughDialog({
  discarding,
  onConfirm,
  onOpenChange,
  open,
}: DiscardWalkthroughDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent data-testid="walkthrough-discard-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>Discard walkthrough?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes the saved walkthrough from this task. The agent can create a new one later.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer" disabled={discarding}>
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            disabled={discarding}
            onClick={onConfirm}
          >
            Discard walkthrough
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
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
  const [confirmDiscardOpen, setConfirmDiscardOpen] = useState(false);
  const [discarding, setDiscarding] = useState(false);
  const { toast } = useToast();

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
    setOpen(true);
  };
  const closeTour = () => {
    clearOpenWalkthroughTaskId(taskId);
    setOpen(false);
  };
  const confirmDiscard = async (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    if (!taskId || discarding) return;
    setDiscarding(true);
    try {
      await deleteTaskWalkthrough(taskId);
      clearOpenWalkthroughTaskId(taskId);
      setOpen(false);
      setWalkthrough(taskId, null);
      setConfirmDiscardOpen(false);
      toast({ title: "Walkthrough discarded", variant: "success" });
    } catch (error) {
      console.error("Failed to discard walkthrough:", error);
      toast({ title: "Failed to discard walkthrough", variant: "error" });
    } finally {
      setDiscarding(false);
    }
  };

  return (
    <>
      {open ? <WalkthroughFloatingWindow onClose={closeTour} onSelectFile={onSelectFile} /> : null}
      <WalkthroughLauncher
        activeStep={activeStep}
        hasUnseen={hasUnseen}
        isOpen={open}
        onDiscardClick={() => setConfirmDiscardOpen(true)}
        onToggle={() => (open ? closeTour() : openTour())}
        stepCount={walkthrough.steps.length}
      />
      <DiscardWalkthroughDialog
        discarding={discarding}
        onConfirm={confirmDiscard}
        onOpenChange={setConfirmDiscardOpen}
        open={confirmDiscardOpen}
      />
    </>
  );
}
