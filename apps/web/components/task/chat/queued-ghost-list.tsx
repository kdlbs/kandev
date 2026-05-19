"use client";

import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { IconLayoutList, IconTrash, IconX } from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@kandev/ui";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useQueue } from "@/hooks/domains/session/use-queue";
import { QueuedGhostMessage } from "./queued-ghost-message";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const HEAD_PREVIEW_MAX = 80;

/** Inter-task entries are dispatched with queued_by="agent" and stay read-only. */
function canUserEditEntry(entry: QueuedMessage): boolean {
  return entry.queued_by !== "agent";
}

function stripSystemTags(text: string): string {
  return text.replace(/<kandev-system>[\s\S]*?<\/kandev-system>/g, "").trim();
}

function headPreviewText(entries: QueuedMessage[]): string {
  const first = entries[0];
  if (!first) return "";
  const clean = stripSystemTags(first.content);
  if (clean.length <= HEAD_PREVIEW_MAX) return clean;
  return clean.slice(0, HEAD_PREVIEW_MAX).trimEnd() + "…";
}

function useEscToClose(open: boolean, onClose: () => void): void {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      const t = e.target;
      // Don't hijack Esc while the user is editing inside the queue textarea or
      // any other input on the page (clarification overlay, etc.).
      if (t instanceof HTMLElement && (t.tagName === "INPUT" || t.tagName === "TEXTAREA")) return;
      e.preventDefault();
      onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
}

type QueueAffordanceProps = {
  sessionId: string | null;
  children: ReactNode;
};

/**
 * Wraps the chat input with the per-session queue affordance:
 * - When there are no queued entries, just renders `children` (the input).
 * - Otherwise a small floating "n queued" chip sits over the input frame and
 *   clicking it expands a panel above the input. Drained or session-switched
 *   queues auto-collapse.
 */
export function QueueAffordance({ sessionId, children }: QueueAffordanceProps) {
  const { entries, count, max, isFull, clearAll, editEntry, removeEntry } = useQueue(sessionId);
  const [isOpen, setIsOpen] = useState(false);

  // Reset disclosure on session switch or full drain using render-phase state
  // adjustment (React docs: "Adjusting some state when a prop changes"). This
  // avoids the cascading-render anti-pattern of doing it inside useEffect.
  const entryCount = entries.length;
  const [lastSession, setLastSession] = useState(sessionId);
  const [lastEntryCount, setLastEntryCount] = useState(entryCount);
  if (sessionId !== lastSession) {
    setLastSession(sessionId);
    setIsOpen(false);
  }
  if (entryCount !== lastEntryCount) {
    setLastEntryCount(entryCount);
    if (entryCount === 0) setIsOpen(false);
  }

  const close = useCallback(() => setIsOpen(false), []);
  useEscToClose(isOpen, close);

  const handleSave = useCallback(
    async (entryId: string, content: string) => {
      await editEntry(entryId, content);
    },
    [editEntry],
  );

  const handleRemove = useCallback(
    async (entryId: string) => {
      try {
        await removeEntry(entryId);
      } catch (err) {
        console.error("Failed to remove queued entry:", err);
        toast.error("Failed to remove queued message.");
      }
    },
    [removeEntry],
  );

  const handleClear = useCallback(() => {
    clearAll().catch((err) => {
      console.error("Failed to clear queued messages:", err);
      toast.error("Failed to clear queued messages.");
    });
  }, [clearAll]);

  if (!sessionId || entryCount === 0) return <>{children}</>;

  return (
    <>
      <Collapsible open={isOpen} onOpenChange={setIsOpen}>
        <CollapsibleContent
          className={cn(
            "overflow-hidden",
            "data-[state=open]:animate-queue-open data-[state=closed]:animate-queue-close",
          )}
        >
          <QueuePanel
            entries={entries}
            count={count}
            max={max}
            isFull={isFull}
            onClose={close}
            onClear={handleClear}
            onSave={handleSave}
            onRemove={handleRemove}
          />
        </CollapsibleContent>
      </Collapsible>
      {!isOpen && (
        <div className="flex items-center px-1 pb-1 animate-in fade-in-0 slide-in-from-bottom-1 duration-150">
          <QueueChip
            count={count}
            isFull={isFull}
            isOpen={isOpen}
            previewText={headPreviewText(entries)}
            onToggle={() => setIsOpen((v) => !v)}
          />
        </div>
      )}
      {children}
    </>
  );
}

type QueueChipProps = {
  count: number;
  isFull: boolean;
  isOpen: boolean;
  previewText: string;
  onToggle: () => void;
};

function chipPalette(isOpen: boolean, isFull: boolean): string {
  if (isOpen) {
    return "text-blue-600 dark:text-blue-300 border-blue-500/50 bg-blue-500/10";
  }
  if (isFull) {
    return "text-amber-600 dark:text-amber-400 border-amber-500/40 hover:bg-amber-500/10";
  }
  return "text-muted-foreground border-border hover:text-foreground hover:border-border/80";
}

function QueueChip({ count, isFull, isOpen, previewText, onToggle }: QueueChipProps) {
  const button = (
    <button
      type="button"
      data-testid="queue-chip"
      data-open={isOpen ? "true" : "false"}
      data-full={isFull ? "true" : "false"}
      aria-expanded={isOpen}
      aria-controls="queue-panel"
      aria-label={`${count} queued message${count === 1 ? "" : "s"}`}
      onClick={onToggle}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5",
        "text-[11px] font-medium cursor-pointer transition-colors",
        chipPalette(isOpen, isFull),
      )}
    >
      <IconLayoutList className="h-3 w-3" />
      <span>{count} queued</span>
      {isFull && <span className="opacity-80">· full</span>}
    </button>
  );
  if (!previewText || isOpen) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent side="top" align="start" className="max-w-[280px]">
        <span className="line-clamp-2">{previewText}</span>
      </TooltipContent>
    </Tooltip>
  );
}

type QueuePanelProps = {
  entries: QueuedMessage[];
  count: number;
  max: number;
  isFull: boolean;
  onClose: () => void;
  onClear: () => void;
  onSave: (entryId: string, content: string) => Promise<void>;
  onRemove: (entryId: string) => Promise<void>;
};

function QueuePanel({
  entries,
  count,
  max,
  isFull,
  onClose,
  onClear,
  onSave,
  onRemove,
}: QueuePanelProps) {
  return (
    <div
      id="queue-panel"
      role="region"
      aria-label="Queued messages"
      data-testid="queued-ghost-list"
      className={cn(
        "flex-shrink-0 px-3 pt-1.5 pb-1 border-t border-border/40",
        "animate-in slide-in-from-bottom-2 fade-in-0 duration-200",
      )}
    >
      <QueuePanelHeader
        count={count}
        max={max}
        isFull={isFull}
        onClear={onClear}
        onClose={onClose}
      />
      <div className="space-y-1.5">
        {entries.map((entry, index) => (
          <QueuedGhostMessage
            key={entry.id}
            entry={entry}
            index={index}
            canEdit={canUserEditEntry(entry)}
            onSave={(content) => onSave(entry.id, content)}
            onRemove={() => onRemove(entry.id)}
          />
        ))}
      </div>
    </div>
  );
}

type QueuePanelHeaderProps = {
  count: number;
  max: number;
  isFull: boolean;
  onClear: () => void;
  onClose: () => void;
};

function QueuePanelHeader({ count, max, isFull, onClear, onClose }: QueuePanelHeaderProps) {
  const capacityText = max > 0 ? `${count} of ${max}` : `${count}`;
  return (
    <div className="flex items-center justify-between gap-3 py-1">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <IconLayoutList className="h-3.5 w-3.5" />
        <span className="uppercase tracking-wide">Queued</span>
        <span className={cn(isFull && "text-amber-600 dark:text-amber-400")}>
          {capacityText}
          {isFull ? " · full" : ""}
        </span>
      </div>
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
          onClick={onClear}
          title="Clear all queued messages"
          data-testid="queue-clear-all"
        >
          <IconTrash className="mr-1 h-3 w-3" />
          Clear all
        </Button>
        <button
          type="button"
          onClick={onClose}
          aria-label="Collapse queued messages"
          data-testid="queue-close"
          className="text-muted-foreground hover:text-foreground cursor-pointer rounded p-1"
        >
          <IconX className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}
