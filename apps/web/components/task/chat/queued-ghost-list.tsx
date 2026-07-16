"use client";

import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { IconLayoutList, IconPlayerPlay, IconTrash, IconX } from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@kandev/ui";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { stripSystemTags } from "@/lib/utils/system-tags";
import { useQueue } from "@/hooks/domains/session/use-queue";
import { isWorkflowQueuedMessage, QueuedGhostMessage } from "./queued-ghost-message";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const HEAD_PREVIEW_MAX = 80;

/** Inter-task entries are dispatched with queued_by="agent" and stay read-only. */
function canUserEditEntry(entry: QueuedMessage): boolean {
  return entry.queued_by !== "agent" && !isWorkflowQueuedMessage(entry);
}

function headPreviewText(entries: QueuedMessage[]): string {
  const first = entries[0];
  if (!first) return "";
  const clean = stripSystemTags(first.content);
  if (clean.length <= HEAD_PREVIEW_MAX) return clean;
  return clean.slice(0, HEAD_PREVIEW_MAX).trimEnd() + "…";
}

function isEditableTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  const tag = el.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  // contentEditable covers the TipTap chat editor and any rich-text surface.
  // `el.isContentEditable` walks up the contenteditable inheritance chain.
  return el.isContentEditable;
}

function useEscToClose(open: boolean, onClose: () => void): void {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      // Don't hijack Esc while the user is editing inside any input control on
      // the page (queue textarea, TipTap chat editor, native input/select, or
      // the clarification overlay).
      if (isEditableTarget(e.target)) return;
      e.preventDefault();
      e.stopPropagation();
      onClose();
    };
    // Capture Escape before an enclosing Radix dialog handles it as a dismissal.
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [open, onClose]);
}

type QueueAffordanceProps = {
  sessionId: string | null;
  children: ReactNode;
  canDrain?: boolean;
  /**
   * Optional render slot for placing the chip inside an external row (e.g. the
   * chat status bar). The callback receives the chip node (or `null` when no
   * chip should be shown) and returns the row markup. When omitted, the chip
   * falls back to rendering inline above the input.
   */
  renderStatusBar?: (queueChip: ReactNode) => ReactNode;
};

type QueuePanelHandlerArgs = {
  clearAll: () => Promise<void>;
  drainNext: () => Promise<void>;
  editEntry: (entryId: string, content: string) => Promise<void>;
  removeEntry: (entryId: string) => Promise<void>;
};

function useQueuePanelHandlers({
  clearAll,
  drainNext,
  editEntry,
  removeEntry,
}: QueuePanelHandlerArgs) {
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
  const handleDrain = useCallback(() => {
    drainNext().catch((err) => {
      console.error("Failed to run queued message:", err);
      toast.error("Failed to run queued message.");
    });
  }, [drainNext]);

  return { handleSave, handleRemove, handleClear, handleDrain };
}

/**
 * Wraps the chat input with the per-session queue affordance:
 * - When there are no queued entries, just renders `children` (the input).
 * - Otherwise a small "n queued" chip is exposed (either inline above the
 *   input, or via `renderStatusBar` into a caller-controlled row); clicking
 *   it expands a panel above the input. Drained or session-switched queues
 *   auto-collapse.
 */
export function QueueAffordance({
  sessionId,
  children,
  canDrain = false,
  renderStatusBar,
}: QueueAffordanceProps) {
  const { entries, count, max, isFull, isLoading, clearAll, drainNext, editEntry, removeEntry } =
    useQueue(sessionId);
  const [isOpen, setIsOpen] = useState(false);
  const { handleSave, handleRemove, handleClear, handleDrain } = useQueuePanelHandlers({
    clearAll,
    drainNext,
    editEntry,
    removeEntry,
  });

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

  const hasEntries = !!sessionId && entryCount > 0;
  const chipNode =
    hasEntries && !isOpen ? (
      <QueueChip
        count={count}
        isFull={isFull}
        previewText={headPreviewText(entries)}
        onToggle={() => setIsOpen((v) => !v)}
      />
    ) : null;

  if (!hasEntries) {
    return (
      <>
        {/* Call renderStatusBar even with null so the status bar stays mounted when the queue is empty. */}
        {renderStatusBar?.(null)}
        {children}
      </>
    );
  }

  return (
    <>
      {renderStatusBar?.(chipNode)}
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
            canDrain={canDrain}
            isLoading={isLoading}
            onClose={close}
            onClear={handleClear}
            onDrain={handleDrain}
            onSave={handleSave}
            onRemove={handleRemove}
          />
        </CollapsibleContent>
      </Collapsible>
      {!renderStatusBar && chipNode && (
        <div className="flex items-center px-1 pb-1 animate-in fade-in-0 slide-in-from-bottom-1 duration-150 motion-reduce:animate-none">
          {chipNode}
        </div>
      )}
      {children}
    </>
  );
}

type QueueChipProps = {
  count: number;
  isFull: boolean;
  previewText: string;
  onToggle: () => void;
};

function chipPalette(isFull: boolean): string {
  if (isFull) {
    return "text-amber-600 dark:text-amber-400 border-amber-500/60 hover:bg-amber-500/10";
  }
  return "text-muted-foreground border-muted-foreground/40 hover:text-foreground hover:border-muted-foreground/60";
}

function QueueChip({ count, isFull, previewText, onToggle }: QueueChipProps) {
  // aria-expanded/aria-controls are intentionally omitted: the chip and the
  // expanded panel are mutually exclusive in the DOM (clicking the chip swaps
  // it for the panel header, which carries its own collapse controls). Pointing
  // aria-controls at an element that isn't currently mounted would be a worse
  // a11y signal than the descriptive aria-label below.
  const button = (
    <button
      type="button"
      data-testid="queue-chip"
      data-full={isFull ? "true" : "false"}
      aria-label={`${count} queued message${count === 1 ? "" : "s"}, click to expand`}
      onClick={onToggle}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5",
        "text-[11px] font-medium cursor-pointer transition-colors",
        chipPalette(isFull),
      )}
    >
      <IconLayoutList className="h-3 w-3" />
      <span>{count} queued</span>
      {isFull && <span className="opacity-80">· full</span>}
    </button>
  );
  if (!previewText) return button;
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
  canDrain: boolean;
  isLoading: boolean;
  onClose: () => void;
  onClear: () => void;
  onDrain: () => void;
  onSave: (entryId: string, content: string) => Promise<void>;
  onRemove: (entryId: string) => Promise<void>;
};

function QueuePanel({
  entries,
  count,
  max,
  isFull,
  canDrain,
  isLoading,
  onClose,
  onClear,
  onDrain,
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
        canDrain={canDrain}
        isLoading={isLoading}
        onClear={onClear}
        onDrain={onDrain}
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
  canDrain: boolean;
  isLoading: boolean;
  onClear: () => void;
  onDrain: () => void;
  onClose: () => void;
};

function QueuePanelHeader({
  count,
  max,
  isFull,
  canDrain,
  isLoading,
  onClear,
  onDrain,
  onClose,
}: QueuePanelHeaderProps) {
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
        {canDrain && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={onDrain}
            disabled={isLoading}
            title="Run next queued message"
            data-testid="queue-drain-next"
          >
            <IconPlayerPlay className="mr-1 h-3 w-3" />
            Run next
          </Button>
        )}
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
