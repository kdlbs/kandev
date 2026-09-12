/* eslint-disable max-lines -- queue panel keeps its responsive controls and row composition together. */
"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { IconLayoutList } from "@tabler/icons-react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { toast } from "@/lib/toast/sonner";
import { useTranslation } from "react-i18next";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { stripSystemTags } from "@/lib/utils/system-tags";
import {
  MergeReferenceOverflowError,
  QueueEditConflictError,
  QueueEntryNotFoundError,
  QueueReorderError,
  QueueSendNowError,
} from "@/lib/api/domains/queue-api";
import { useQueue, type MessageAttachment } from "@/hooks/domains/session/use-queue";
import { useQueueEditProtection } from "@/hooks/use-queue-edit-protection";
import { useQueuePinned } from "@/hooks/use-queue-pinned";
import { canMergeWithAbove, QueuedGhostMessage } from "./queued-ghost-message";
import { useQueuePanelOpenState } from "./use-queue-panel-open-state";
import { useQueuePanelEscape } from "./use-queue-panel-escape";
import { QueuePanelHeader } from "./queued-ghost-panel-header";
import type { EntityReference } from "@/lib/types/entity-reference";
import type { QueueEditLease } from "@/lib/api/domains/queue-api";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const HEAD_PREVIEW_MAX = 80;

/** Only WebSocket-authored user rows use the editable `user` provenance. */
function canUserEditEntry(entry: QueuedMessage): boolean {
  return entry.queued_by === "user";
}

function headPreviewText(entries: QueuedMessage[]): string {
  const first = entries[0];
  if (!first) return "";
  const clean = stripSystemTags(first.content);
  if (clean.length <= HEAD_PREVIEW_MAX) return clean;
  return clean.slice(0, HEAD_PREVIEW_MAX).trimEnd() + "…";
}

type QueueAffordanceProps = {
  sessionId: string | null;
  children: ReactNode;
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
  editEntry: (
    entryId: string,
    content: string,
    attachments?: MessageAttachment[],
    entityReferences?: EntityReference[],
    lease?: QueueEditLease | null,
  ) => Promise<void>;
  removeEntry: (entryId: string) => Promise<void>;
  mergeEntry: (entryId: string) => Promise<void>;
  reorderEntries: (orderedIds: string[]) => Promise<void>;
  sendEntryNow: (entryId: string) => Promise<void>;
  setAutoRun: (enabled: boolean) => Promise<void>;
  setAutoMerge: (enabled: boolean) => Promise<void>;
};

function useSendNowPanelHandlers(sendEntryNow: (entryId: string) => Promise<void>) {
  const { t } = useTranslation();
  const sendNowErrorMessage = useCallback(
    (err: unknown) => {
      if (err instanceof QueueEntryNotFoundError) return t("chat:sendNowEntryAlreadySent");
      if (err instanceof QueueEditConflictError) return t("chat:queueEditSaveFailed");
      if (err instanceof QueueSendNowError) {
        const messages: Record<string, string> = {
          queue_empty: "chat:sendNowQueueEmpty",
          queue_changed: "chat:sendNowQueueChanged",
          send_now_conflict: "chat:sendNowConflict",
          turn_changed: "chat:sendNowTurnChanged",
          send_now_attachment_overflow: "chat:sendNowAttachmentOverflow",
          send_now_reference_overflow: "chat:sendNowReferenceOverflow",
        };
        const key = messages[err.code];
        if (key) return t(key);
      }
      return t("chat:failedToSendQueuedMessageNow");
    },
    [t],
  );
  const handleSendEntryNow = useCallback(
    (entryId: string) => {
      sendEntryNow(entryId).catch((err) => {
        console.error("Failed to send queued message now:", err);
        toast.error(sendNowErrorMessage(err));
      });
    },
    [sendEntryNow, sendNowErrorMessage],
  );
  return { handleSendEntryNow };
}

// eslint-disable-next-line max-lines-per-function -- panel actions share queue error and admission handling.
function useQueuePanelHandlers({
  clearAll,
  editEntry,
  removeEntry,
  mergeEntry,
  reorderEntries,
  sendEntryNow,
  setAutoRun,
  setAutoMerge,
}: QueuePanelHandlerArgs) {
  // Tracks merge requests still in flight so a rapid second click on the same
  // row cannot fire a second request for an entry that is already gone — the
  // first click's success refetch removes the row and the second would surface
  // a spurious "not found" error.
  const pendingMerges = useRef(new Set<string>());
  const { t } = useTranslation();
  const { handleSendEntryNow } = useSendNowPanelHandlers(sendEntryNow);
  const handleSave = useCallback(
    async (
      entryId: string,
      content: string,
      attachments: MessageAttachment[] | undefined,
      entityReferences: EntityReference[],
      lease?: QueueEditLease | null,
    ) => {
      if (!lease) {
        toast.error(t("chat:queueEditSaveFailed"));
        return;
      }
      await editEntry(entryId, content, attachments, entityReferences, lease);
    },
    [editEntry, t],
  );
  const handleRemove = useCallback(
    async (entryId: string) => {
      try {
        await removeEntry(entryId);
      } catch (err) {
        console.error("Failed to remove queued entry:", err);
        toast.error(t("chat:failedToRemoveQueuedMessage"));
      }
    },
    [removeEntry, t],
  );
  const handleMerge = useCallback(
    async (entryId: string) => {
      if (pendingMerges.current.has(entryId)) return;
      pendingMerges.current.add(entryId);
      try {
        await mergeEntry(entryId);
      } catch (err) {
        // A drain race (QueueEntryNotFoundError) already triggered a refetch in
        // mergeEntry, so the queue view is resynced — no toast needed for the
        // benign, self-recovering case.
        if (err instanceof QueueEntryNotFoundError) return;
        if (err instanceof MergeReferenceOverflowError) {
          toast.error(t("chat:mergeReferenceOverflow"));
          return;
        }
        console.error("Failed to merge queued entry:", err);
        toast.error(t("chat:failedToMergeQueuedMessages"));
      } finally {
        pendingMerges.current.delete(entryId);
      }
    },
    [mergeEntry],
  );
  const handleClear = useCallback(() => {
    clearAll().catch((err) => {
      console.error("Failed to clear queued messages:", err);
      toast.error(t("chat:failedToClearQueuedMessages"));
    });
  }, [clearAll, t]);
  const handleAutoRunChange = useCallback(
    (enabled: boolean) => {
      setAutoRun(enabled).catch((err) => {
        console.error("Failed to update queue Auto-run:", err);
        toast.error(t("chat:failedToSetQueueAutoRun"));
      });
    },
    [setAutoRun, t],
  );
  const handleAutoMergeChange = useCallback(
    (enabled: boolean) => {
      setAutoMerge(enabled).catch((err) => {
        console.error("Failed to update queue Auto-merge:", err);
        toast.error(t("chat:failedToSetQueueAutoMerge"));
      });
    },
    [setAutoMerge, t],
  );
  const handleReorder = useCallback(
    async (orderedIds: string[]) => {
      try {
        await reorderEntries(orderedIds);
      } catch (err) {
        // A queue_changed race already refetched and reconciled to the
        // authoritative order, so it is a successful UI outcome — no toast.
        if (err instanceof QueueReorderError) return;
        console.error("Failed to reorder queued entries:", err);
        toast.error(t("chat:failedToReorderQueuedMessages"));
      }
    },
    [reorderEntries, t],
  );

  return {
    handleSave,
    handleRemove,
    handleMerge,
    handleClear,
    handleAutoRunChange,
    handleAutoMergeChange,
    handleReorder,
    handleSendEntryNow,
  };
}

type QueuePanelDisclosureProps = {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  entries: QueuedMessage[];
  count: number;
  max: number;
  isFull: boolean;
  autoRun: boolean;
  autoMerge: boolean;
  autoMergeAvailable: boolean;
  isLoading: boolean;
  cancellationPending: boolean;
  mergeEnabled: boolean;
  pinned: boolean;
  editingEntryId: string | null;
  editLeaseActive: boolean;
  onClose: () => void;
  onClear: () => void;
  onAutoRunChange: (enabled: boolean) => void;
  onAutoMergeChange: (enabled: boolean) => void;
  onTogglePin: () => void;
  onSave: (
    entryId: string,
    content: string,
    refs: EntityReference[],
    attachments?: MessageAttachment[],
    lease?: QueueEditLease | null,
  ) => Promise<void>;
  onRemove: (entryId: string) => Promise<void>;
  onMerge: (entryId: string) => Promise<void>;
  onReorder: (orderedIds: string[]) => void;
  onSendEntryNow: (entryId: string) => void;
  onEditStart: (entryId: string) => Promise<string | false>;
  onEditComplete: (entryId: string, editToken?: string, saved?: boolean) => Promise<void>;
};

/** Wraps QueuePanel in the collapsible open/close animation shell. */
function QueuePanelDisclosure({
  isOpen,
  onOpenChange,
  entries,
  count,
  max,
  isFull,
  autoRun,
  autoMerge,
  autoMergeAvailable,
  isLoading,
  cancellationPending,
  mergeEnabled,
  pinned,
  editingEntryId,
  editLeaseActive,
  onClose,
  onClear,
  onAutoRunChange,
  onAutoMergeChange,
  onTogglePin,
  onSave,
  onRemove,
  onMerge,
  onReorder,
  onSendEntryNow,
  onEditStart,
  onEditComplete,
}: QueuePanelDisclosureProps) {
  return (
    <Collapsible open={isOpen} onOpenChange={onOpenChange}>
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
          autoRun={autoRun}
          autoMerge={autoMerge}
          autoMergeAvailable={autoMergeAvailable}
          isLoading={isLoading}
          cancellationPending={cancellationPending}
          editingEntryId={editingEntryId}
          editLeaseActive={editLeaseActive}
          mergeEnabled={mergeEnabled}
          pinned={pinned}
          onClose={onClose}
          onClear={onClear}
          onAutoRunChange={onAutoRunChange}
          onAutoMergeChange={onAutoMergeChange}
          onTogglePin={onTogglePin}
          onSave={onSave}
          onRemove={onRemove}
          onMerge={onMerge}
          onReorder={onReorder}
          onSendEntryNow={onSendEntryNow}
          onEditStart={onEditStart}
          onEditComplete={onEditComplete}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

/**
 * Wraps the chat input with the per-session queue affordance:
 * - When there are no queued entries, just renders `children` (the input).
 * - Otherwise a small "n queued" chip is exposed (either inline above the
 *   input, or via `renderStatusBar` into a caller-controlled row); clicking
 *   it expands a panel above the input. Drained or session-switched queues
 *   auto-collapse.
 */
// eslint-disable-next-line max-lines-per-function -- coordinates the queue panel's responsive controls and mutations.
export function QueueAffordance({ sessionId, children, renderStatusBar }: QueueAffordanceProps) {
  const {
    entries,
    count,
    max,
    isFull,
    mergeEnabled,
    autoRun,
    autoMerge,
    autoMergeAvailable,
    isLoading,
    clearAll,
    setAutoRun,
    setAutoMerge,
    editEntry,
    removeEntry,
    mergeEntry,
    reorderEntries,
    sendEntryNow,
    cancellationPending,
  } = useQueue(sessionId);
  const { editingEntryId, editLease, beginEdit, completeEdit } = useQueueEditProtection({
    sessionId,
    entries,
  });
  const { value: pinned, toggle: togglePin } = useQueuePinned(sessionId);
  const [isOpen, setIsOpen] = useQueuePanelOpenState(sessionId, entries.length, pinned);
  const {
    handleSave: handlePanelSave,
    handleRemove,
    handleMerge,
    handleClear,
    handleAutoRunChange,
    handleAutoMergeChange,
    handleReorder,
    handleSendEntryNow,
  } = useQueuePanelHandlers({
    clearAll,
    editEntry,
    removeEntry,
    mergeEntry,
    reorderEntries,
    sendEntryNow,
    setAutoRun,
    setAutoMerge,
  });

  const handleSave = useCallback(
    (
      entryId: string,
      content: string,
      refs: EntityReference[],
      attachments?: MessageAttachment[],
    ) => handlePanelSave(entryId, content, attachments, refs, editLease),
    [editLease, handlePanelSave],
  );
  const handleEditComplete = useCallback(
    (entryId: string, editToken?: string, saved = false) =>
      completeEdit(entryId, sessionId, editToken, saved),
    [completeEdit, sessionId],
  );
  const close = useCallback(() => setIsOpen(false), []);
  useQueuePanelEscape(isOpen, close);

  const chipNode =
    !!sessionId && entries.length > 0 && !isOpen ? (
      <QueueChip
        count={count}
        isFull={isFull}
        previewText={headPreviewText(entries)}
        onToggle={() => setIsOpen((v) => !v)}
      />
    ) : null;

  if (!sessionId || entries.length === 0) {
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
      <QueuePanelDisclosure
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        entries={entries}
        count={count}
        editingEntryId={editingEntryId}
        max={max}
        isFull={isFull}
        autoRun={autoRun}
        autoMerge={autoMerge}
        autoMergeAvailable={autoMergeAvailable}
        isLoading={isLoading}
        cancellationPending={cancellationPending}
        mergeEnabled={mergeEnabled}
        pinned={pinned}
        onClose={close}
        editLeaseActive={editLease !== null}
        onEditStart={beginEdit}
        onEditComplete={handleEditComplete}
        onClear={handleClear}
        onAutoRunChange={handleAutoRunChange}
        onAutoMergeChange={handleAutoMergeChange}
        onTogglePin={togglePin}
        onSave={handleSave}
        onRemove={handleRemove}
        onMerge={handleMerge}
        onReorder={handleReorder}
        onSendEntryNow={handleSendEntryNow}
      />
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
  const { t } = useTranslation();
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
      aria-label={t("chat:queueChipExpand", { count })}
      onClick={onToggle}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5",
        "text-[11px] font-medium cursor-pointer transition-colors",
        "[@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:px-3",
        chipPalette(isFull),
      )}
    >
      <IconLayoutList className="h-3 w-3" />
      <span>{t("chat:queuedCount", { count })}</span>
      {isFull && <span className="opacity-80">{t("chat:queueFullSuffix")}</span>}
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
  autoRun: boolean;
  autoMerge: boolean;
  autoMergeAvailable: boolean;
  isLoading: boolean;
  cancellationPending: boolean;
  mergeEnabled: boolean;
  pinned: boolean;
  editingEntryId: string | null;
  editLeaseActive: boolean;
  onClose: () => void;
  onClear: () => void;
  onAutoRunChange: (enabled: boolean) => void;
  onAutoMergeChange: (enabled: boolean) => void;
  onTogglePin: () => void;
  onSave: (
    entryId: string,
    content: string,
    refs: EntityReference[],
    attachments?: MessageAttachment[],
    lease?: QueueEditLease | null,
  ) => Promise<void>;
  onRemove: (entryId: string) => Promise<void>;
  onMerge: (entryId: string) => Promise<void>;
  onReorder: (orderedIds: string[]) => void;
  onSendEntryNow: (entryId: string) => void;
  onEditStart: (entryId: string) => Promise<string | false>;
  onEditComplete: (entryId: string, editToken?: string, saved?: boolean) => Promise<void>;
};
/** Renders the expanded queue list: header controls plus one QueuedGhostMessage
 * row per pending entry, gating each row's merge control on `mergeEnabled`. */
type QueueReorderArgs = {
  entries: QueuedMessage[];
  canReorder: boolean;
  onReorder: (orderedIds: string[]) => void;
};

/** Sortable-list state for the queue panel: sensors, the id list, and the
 * drag lifecycle handlers that translate a drop into the new ordered ids. */
function useQueueReorder({ entries, canReorder, onReorder }: QueueReorderArgs) {
  const [activeId, setActiveId] = useState<string | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const ids = useMemo(() => entries.map((entry) => entry.id), [entries]);

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveId(String(event.active.id));
  }, []);

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      setActiveId(null);
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const oldIndex = entries.findIndex((entry) => entry.id === active.id);
      const newIndex = entries.findIndex((entry) => entry.id === over.id);
      if (oldIndex === -1 || newIndex === -1) return;
      onReorder(arrayMove(ids, oldIndex, newIndex));
    },
    [entries, ids, onReorder],
  );

  const handleDragCancel = useCallback(() => setActiveId(null), []);

  return {
    ids,
    sensors,
    activeId,
    canReorder,
    handleDragStart,
    handleDragEnd,
    handleDragCancel,
  };
}

// Reordering is disabled while a queue mutation or backend cancellation is in flight.
type QueuePanelEntryProps = {
  entry: QueuedMessage;
  index: number;
  editLeaseActive: boolean;
  canEdit: boolean;
  canMerge: boolean;
  canDrag: boolean;
  showDragHandle: boolean;
  isDragging: boolean;
  sendNowDisabled: boolean;
  onSave: QueuePanelProps["onSave"];
  onRemove: () => Promise<void>;
  onMerge: () => Promise<void>;
  onSendNow: () => void;
  onEditStart: () => Promise<string | false>;
  onEditComplete: (editToken?: string, saved?: boolean) => Promise<void>;
};

function QueuePanelEntry({
  entry,
  index,
  editLeaseActive,
  canEdit,
  canMerge,
  canDrag,
  showDragHandle,
  isDragging,
  sendNowDisabled,
  onSave,
  onRemove,
  onMerge,
  onSendNow,
  onEditStart,
  onEditComplete,
}: QueuePanelEntryProps) {
  return (
    <QueuedGhostMessage
      entry={entry}
      index={index}
      editLeaseActive={editLeaseActive}
      canEdit={canEdit}
      canRemove
      canMerge={canMerge}
      canDrag={canDrag}
      showDragHandle={showDragHandle}
      isDragging={isDragging}
      onSave={(content, entityReferences, attachments) =>
        onSave(entry.id, content, entityReferences, attachments)
      }
      onRemove={onRemove}
      onMerge={onMerge}
      onSendNow={onSendNow}
      sendNowDisabled={sendNowDisabled}
      onEditStart={onEditStart}
      onEditComplete={onEditComplete}
    />
  );
}

// eslint-disable-next-line max-lines-per-function -- the panel keeps responsive drag, edit, and status composition together.
function QueuePanel({
  entries,
  count,
  max,
  isFull,
  autoRun,
  autoMerge,
  autoMergeAvailable,
  isLoading,
  cancellationPending,
  mergeEnabled,
  pinned,
  editingEntryId,
  editLeaseActive,
  onClose,
  onClear,
  onAutoRunChange,
  onAutoMergeChange,
  onTogglePin,
  onSave,
  onRemove,
  onMerge,
  onReorder,
  onSendEntryNow,
  onEditStart,
  onEditComplete,
}: QueuePanelProps) {
  const { t } = useTranslation();
  // The server remains authoritative; drops reconcile through refetch.
  const { ids, sensors, activeId, canReorder, handleDragStart, handleDragEnd, handleDragCancel } =
    useQueueReorder({
      entries,
      canReorder: !isLoading && !cancellationPending,
      onReorder,
    });

  return (
    <div
      id="queue-panel"
      role="region"
      aria-label={t("chat:queuedMessages")}
      data-testid="queued-ghost-list"
      className={cn(
        "flex max-h-[min(40dvh,32rem)] flex-shrink-0 flex-col px-3 pt-1.5 pb-1",
        "border-t border-border/40",
        "animate-in slide-in-from-bottom-2 fade-in-0 duration-200",
      )}
    >
      <QueuePanelHeader
        count={count}
        max={max}
        isFull={isFull}
        autoRun={autoRun}
        autoMerge={autoMerge}
        autoMergeAvailable={autoMergeAvailable}
        isLoading={isLoading || editingEntryId !== null}
        cancellationPending={cancellationPending}
        pinned={pinned}
        onClear={onClear}
        onAutoRunChange={onAutoRunChange}
        onAutoMergeChange={onAutoMergeChange}
        onTogglePin={onTogglePin}
        onClose={onClose}
      />
      <div
        data-testid="queue-scroll-region"
        className="min-h-0 flex-1 space-y-1.5 overflow-y-auto overscroll-contain pr-1"
      >
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={handleDragStart}
          onDragEnd={handleDragEnd}
          onDragCancel={handleDragCancel}
        >
          <SortableContext items={ids} strategy={verticalListSortingStrategy}>
            {entries.map((entry, index) => (
              <QueuePanelEntry
                key={entry.id}
                entry={entry}
                index={index}
                editLeaseActive={editLeaseActive}
                canEdit={
                  !isLoading &&
                  !cancellationPending &&
                  canUserEditEntry(entry) &&
                  (editingEntryId === null || editingEntryId === entry.id)
                }
                canMerge={mergeEnabled && canMergeWithAbove(entry, entries[index - 1])}
                canDrag={canReorder}
                showDragHandle={entries.length > 1}
                isDragging={activeId === entry.id}
                sendNowDisabled={isLoading || cancellationPending}
                onSave={onSave}
                onRemove={() => onRemove(entry.id)}
                onMerge={() => onMerge(entry.id)}
                onSendNow={() => onSendEntryNow(entry.id)}
                onEditStart={() => onEditStart(entry.id)}
                onEditComplete={(editToken, saved) => onEditComplete(entry.id, editToken, saved)}
              />
            ))}
          </SortableContext>
        </DndContext>
      </div>
    </div>
  );
}
