"use client";

import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { IconInfoCircle, IconRobot, IconUser } from "@tabler/icons-react";
import ReactMarkdown from "react-markdown";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { DraggableAttributes, DraggableSyntheticListeners } from "@dnd-kit/core";
import { toast } from "@/lib/toast/sonner";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { QueueEntryNotFoundError } from "@/lib/api/domains/queue-api";
import { stripSystemTags } from "@/lib/utils/system-tags";
import {
  SenderTaskBadge,
  type SenderTaskInfo,
} from "@/components/task/chat/messages/sender-task-badge";
import {
  WorkflowStepMessageBadge,
  workflowMessageInfoFromMetadata,
  type WorkflowStepMessageInfo,
} from "@/components/task/chat/messages/workflow-step-message-badge";
import { remarkPlugins } from "@/components/shared/markdown-components";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  entityReferencesFromMetadata,
  survivingEntityReferences,
} from "@/lib/entity-references/message-references";
import { buildEntityReferenceMarkdownComponents } from "@/components/task/chat/messages/entity-reference-chip";
import { QueuedGhostRowActions } from "@/components/task/chat/queued-ghost-row-actions";
import { useQueuedMessageOverflow } from "@/components/task/chat/use-queued-message-overflow";
import { AttachmentRow, type QueuedAttachment } from "@/components/task/chat/queued-attachment-row";
import { QueuedGhostEditView } from "@/components/task/chat/queued-ghost-edit-view";
import { t } from "@/lib/i18n";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import {
  useQueuedGhostLeaseLoss,
  useQueuedGhostStartEdit,
} from "@/hooks/use-queue-edit-protection";

/** Imperative handle for the ghost row, used by chat input "edit last queued" affordance. */
export type QueuedGhostMessageHandle = {
  startEdit: () => void;
};

type SenderKind = "user" | "agent" | "workflow" | "system";

export function isWorkflowQueuedMessage(entry: QueuedMessage): boolean {
  return (
    entry.queued_by === "workflow" ||
    entry.queued_by === "workflow-auto-start" ||
    workflowMessageInfoFromMetadata(entry.metadata) !== null
  );
}

function senderKindOf(entry: QueuedMessage): SenderKind {
  if (isWorkflowQueuedMessage(entry)) return "workflow";
  if (!entry.queued_by) return "system";
  // Inter-task messages dispatched via dispatchTaskMessage hardcode
  // queued_by="agent"; that's the only signal needed.
  if (entry.queued_by === "agent") return "agent";
  return "user";
}

/** A message may serve as a merge source or target only when its sender kind is
 * "user" or "agent". Server-owned rows (queued_by="server") are reserved
 * backend rows and are never mergeable. */
export function canMergeEntry(entry: QueuedMessage): boolean {
  if (entry.queued_by === "server") return false;
  if (
    entry.metadata &&
    ("plan_comment_refs" in entry.metadata ||
      "plan_comment_request_fingerprint" in entry.metadata ||
      "client_queue_id" in entry.metadata)
  ) {
    return false;
  }
  const kind = senderKindOf(entry);
  return kind === "user" || kind === "agent";
}

/** Two messages may merge when both are mergeable and share the same sender
 * kind. Agent entries additionally require an identical non-empty
 * sender_task_id; user entries require both rows to be owned by the caller
 * identity (callerId defaults to the backend's QueuedByUser, which is what the
 * SPA sends when it omits user_id). Mirrors the backend's mergeAllowed
 * predicate so the button never appears for merges the server would reject. */
export function canMergeWithAbove(
  entry: QueuedMessage,
  above: QueuedMessage | undefined,
  callerId = "user",
): boolean {
  if (!above) return false;
  if (!canMergeEntry(entry) || !canMergeEntry(above)) return false;
  if (senderKindOf(entry) !== senderKindOf(above)) return false;
  if (senderKindOf(entry) === "agent") {
    const senderTaskId = getSenderTaskInfo(entry)?.id;
    return senderTaskId !== undefined && senderTaskId === getSenderTaskInfo(above)?.id;
  }
  if (senderKindOf(entry) === "user") {
    return callerId !== "" && entry.queued_by === callerId && above.queued_by === callerId;
  }
  return false;
}

function senderLabel(entry: QueuedMessage): string {
  const kind = senderKindOf(entry);
  if (kind === "agent") {
    const title = entry.metadata?.sender_task_title;
    const sessionName = entry.metadata?.sender_session_name;
    const base =
      typeof title === "string" && title.length > 0
        ? t("task:fromTask", { title })
        : t("task:fromAgent");
    return typeof sessionName === "string" && sessionName.length > 0
      ? t("task:senderWithSession", { base, sessionName })
      : base;
  }
  if (kind === "workflow") return t("task:workflowSender");
  if (kind === "system") return t("task:systemSender");
  return t("task:you");
}

type SenderIconProps = { entry: QueuedMessage };

function senderIconFor(kind: SenderKind): { Icon: typeof IconUser; tone: string } {
  if (kind === "agent") {
    return { Icon: IconRobot, tone: "text-amber-500 dark:text-amber-400" };
  }
  if (kind === "system") {
    return { Icon: IconInfoCircle, tone: "text-muted-foreground" };
  }
  if (kind === "workflow") {
    return { Icon: IconInfoCircle, tone: "text-muted-foreground" };
  }
  return { Icon: IconUser, tone: "text-blue-500 dark:text-blue-300" };
}

function SenderIcon({ entry }: SenderIconProps) {
  const { Icon, tone } = senderIconFor(senderKindOf(entry));
  return <Icon className={cn("h-3.5 w-3.5 flex-shrink-0", tone)} aria-label={senderLabel(entry)} />;
}

/**
 * Returns the inter-task sender info when this entry was queued by another
 * task's agent (queued_by="agent" + sender_task_id in metadata). User-typed
 * entries have no SenderTaskInfo — they render with just the IconUser cue.
 */
function getSenderTaskInfo(entry: QueuedMessage): SenderTaskInfo | null {
  if (senderKindOf(entry) !== "agent") return null;
  const meta = entry.metadata as
    | {
        sender_task_id?: string;
        sender_task_title?: string;
        sender_session_id?: string;
        sender_session_name?: string;
      }
    | undefined;
  const id = meta?.sender_task_id;
  if (typeof id !== "string" || id.length === 0) return null;
  return {
    id,
    snapshotTitle: meta?.sender_task_title ?? "",
    sessionId: meta?.sender_session_id,
    sessionName: meta?.sender_session_name,
  };
}

function getWorkflowMessageInfo(entry: QueuedMessage): WorkflowStepMessageInfo | null {
  const info = workflowMessageInfoFromMetadata(entry.metadata);
  if (info) return info;
  return entry.queued_by === "workflow" || entry.queued_by === "workflow-auto-start" ? {} : null;
}

type DisplayViewProps = {
  entry: QueuedMessage;
  entityReferences: readonly EntityReference[];
  positionLabel: string;
  canEdit: boolean;
  canRemove: boolean;
  canMerge: boolean;
  onStartEdit: () => void;
  onRemove: () => void;
  onMerge?: () => void | Promise<void>;
  onSendNow: () => void;
  sendNowDisabled: boolean;
};

function queuePositionLabel(index: number | undefined, position: number | undefined): string {
  return `#${index === undefined ? (position ?? 1) : index + 1}`;
}

function DisplayView({
  entry,
  entityReferences,
  positionLabel,
  canEdit,
  canRemove,
  canMerge,
  onStartEdit,
  onRemove,
  onMerge,
  onSendNow,
  sendNowDisabled,
}: DisplayViewProps) {
  const { t } = useTranslation();
  const visible = stripSystemTags(entry.content);
  const attachments = (entry.attachments ?? []) as QueuedAttachment[];
  const senderTask = getSenderTaskInfo(entry);
  const workflowMessage = getWorkflowMessageInfo(entry);
  const [expanded, setExpanded] = useState(false);
  const { previewRef, disclosureButtonRef, canExpand } = useQueuedMessageOverflow(
    visible,
    expanded,
    setExpanded,
  );
  const referenceMarkdownComponents = useMemo(
    () => buildEntityReferenceMarkdownComponents(entityReferences),
    [entityReferences],
  );
  return (
    <div className="group flex items-center gap-2 py-1.5" data-testid="queue-entry">
      {/* Position label + sender icon are non-interactive; relative keeps them
       * painted above the coarse-pointer touch target, and pointer-events-none
       * lets touches reach the handle behind them. */}
      <span className="relative flex items-center gap-1.5 text-muted-foreground pointer-events-none">
        <span
          aria-label={t("task:position", { positionLabel })}
          className="font-mono text-[10px] tabular-nums"
        >
          {positionLabel}
        </span>
        {!senderTask && !workflowMessage && <SenderIcon entry={entry} />}
      </span>
      <div className="flex-1 min-w-0 space-y-1">
        {workflowMessage && <WorkflowStepMessageBadge workflow={workflowMessage} size="xs" />}
        {senderTask && <SenderTaskBadge sender={senderTask} size="xs" />}
        {visible && (
          <div
            ref={previewRef}
            data-testid="queue-entry-text"
            data-expanded={expanded ? "true" : "false"}
            className={cn(
              "markdown-body max-w-none text-sm text-foreground/80 break-words overflow-hidden",
              "[&>*:first-child]:mt-0 [&>*:last-child]:mb-0",
              expanded ? "max-h-[40rem]" : "max-h-[2.75rem]",
            )}
          >
            <ReactMarkdown remarkPlugins={remarkPlugins} components={referenceMarkdownComponents}>
              {visible}
            </ReactMarkdown>
          </div>
        )}
        <AttachmentRow attachments={attachments} interactive={true} />
      </div>
      <QueuedGhostRowActions
        canExpand={canExpand}
        disclosureButtonRef={disclosureButtonRef}
        expanded={expanded}
        canMerge={canMerge}
        canEdit={canEdit}
        canRemove={canRemove}
        onToggleExpand={() => setExpanded((v) => !v)}
        onMerge={onMerge}
        onStartEdit={onStartEdit}
        onRemove={onRemove}
        onSendNow={onSendNow}
        sendNowDisabled={sendNowDisabled}
      />
    </div>
  );
}

type QueueGrabHandleProps = {
  canDrag: boolean;
  attributes: DraggableAttributes;
  listeners: DraggableSyntheticListeners;
  /** dnd-kit activator node ref: after a keyboard drag, focus returns here. */
  setActivatorNodeRef: (node: HTMLElement | null) => void;
};

/**
 * Dotted drag handle that floats over the row's left edge. It is absolutely
 * positioned so nothing in the row shifts to make room. It is always visible
 * when the queue has multiple entries, attached flush to the box's left edge
 * and painted behind the row content on coarse pointers (no chip surface —
 * the dots read as part of the box edge).
 * Dragging starts only from this handle.
 */
function QueueGrabHandle({
  canDrag,
  attributes,
  listeners,
  setActivatorNodeRef,
}: QueueGrabHandleProps) {
  const { t } = useTranslation();
  const dragProps = canDrag ? { ...attributes, ...listeners } : {};
  return (
    <button
      type="button"
      ref={setActivatorNodeRef}
      disabled={!canDrag}
      aria-label={t("chat:reorderQueuedMessage")}
      data-testid="queue-grab-handle"
      {...dragProps}
      aria-roledescription={t("chat:sortable")}
      className={cn(
        "absolute top-1/2 z-10 -translate-y-1/2 touch-none rounded-md",
        "flex items-center justify-center",
        "left-1 h-3 w-3",
        "text-muted-foreground/70 hover:text-muted-foreground",
        "border-0 bg-transparent shadow-none",
        "opacity-100 transition-opacity duration-150",
        canDrag ? "cursor-grab active:cursor-grabbing" : "cursor-not-allowed opacity-30",
        // Coarse pointers: keep a 44px touch target while the visible grip
        // remains small and behind the row content.
        "[@media(pointer:coarse)]:opacity-100",
        "[@media(pointer:coarse)]:left-0 [@media(pointer:coarse)]:z-0",
        "[@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11",
        "[@media(pointer:coarse)]:border-0",
        "[@media(pointer:coarse)]:bg-transparent",
        "[@media(pointer:coarse)]:shadow-none",
        "[@media(pointer:coarse)]:justify-start [@media(pointer:coarse)]:pl-2",
      )}
    >
      <span aria-hidden className="grid grid-cols-2 gap-0.5">
        {[0, 1, 2, 3, 4, 5].map((dot) => (
          <span key={dot} className="h-px w-px rounded-full bg-current" />
        ))}
      </span>
    </button>
  );
}

type SortableRowShellProps = {
  id: string;
  /** useSortable is disabled while the row is edited or reordering is off. */
  disabled: boolean;
  canDrag: boolean;
  /** The grab handle is hidden while the row is being edited. */
  showHandle: boolean;
  isDragging: boolean;
  children: ReactNode;
};

/** Sortable row wrapper: owns the dnd-kit node ref, transform, and the dotted
 * grab handle overlay, keeping QueuedGhostMessage focused on content. */
function SortableRowShell({
  id,
  disabled,
  canDrag,
  showHandle,
  isDragging,
  children,
}: SortableRowShellProps) {
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition } =
    useSortable({
      id,
      disabled,
    });
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(
        "group relative rounded-md border border-border/60 bg-background/40 pr-2 text-sm",
        "hover:border-border transition-colors",
        showHandle ? "pl-5 [@media(pointer:coarse)]:pl-11" : "pl-2",
        isDragging && "z-10 opacity-40",
      )}
    >
      {showHandle && (
        <QueueGrabHandle
          canDrag={canDrag}
          attributes={attributes}
          listeners={listeners}
          setActivatorNodeRef={setActivatorNodeRef}
        />
      )}
      {children}
    </div>
  );
}

type QueuedGhostMessageProps = {
  entry: QueuedMessage;
  /** Zero-based render position; combined with entry.position to label the row. */
  index?: number;
  /**
   * Edit is only allowed when the caller's identity (currentUserId) matches the
   * entry's queued_by. Inter-task entries are visible but read-only.
   */
  canEdit: boolean;
  /**
   * Becomes false when the server-side edit lease is lost, so a row cannot
   * continue presenting an editor that can no longer submit.
   */
  editLeaseActive?: boolean;
  /** Removal is independent from edit ownership and applies to every visible row. */
  canRemove?: boolean;
  /**
   * Merge-with-above control; only shown when this row and the one above it
   * share a sender kind. Optional so standalone renders (tests) can omit it.
   */
  canMerge?: boolean;
  /**
   * Reordering is enabled unless a queue mutation or backend cancellation is
   * in flight; the row's own edit view also suppresses the handle. The queue
   * panel hides it when there is only one entry. Optional so standalone renders
   * default to showing the handle.
   */
  canDrag?: boolean;
  /** Whether this row belongs to a queue with enough entries to reorder. */
  showDragHandle?: boolean;
  /** Set while this row is the active drag target, for dimming/z-index. */
  isDragging?: boolean;
  onSave: (
    content: string,
    entityReferences: EntityReference[],
    attachments?: QueuedMessage["attachments"],
  ) => Promise<void>;
  onRemove: () => void | Promise<void>;
  /** Fold this entry into the one above it. */
  onMerge?: () => void | Promise<void>;
  /** Interrupt the current turn and dispatch this exact queued entry. */
  onSendNow?: () => void;
  /** Disable while the queue mutation or backend cancellation is in flight. */
  sendNowDisabled?: boolean;
  /** Called before edit activation; returning false keeps the row read-only. */
  onEditStart?: () => void | Promise<boolean | string | void>;
  /** Called after edit save/cancel so the parent can restore queue policy. */
  onEditComplete?: (editToken?: string, saved?: boolean) => void | Promise<void>;
};

function useFocusQueuedEdit(
  editing: boolean,
  textareaRef: React.RefObject<HTMLTextAreaElement | null>,
): void {
  useQueuedGhostEditEscapeGuard(editing, textareaRef);
  useEffect(() => {
    if (!editing || !textareaRef.current) return;
    const textarea = textareaRef.current;
    textarea.focus();
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
  }, [editing, textareaRef]);
}

function useQueuedGhostEditEscapeGuard(
  editing: boolean,
  textareaRef: React.RefObject<HTMLTextAreaElement | null>,
): void {
  const editEscapeGuard = useCallback(
    (event: KeyboardEvent) => {
      if (!editing || event.key !== "Escape") return false;
      const textarea = textareaRef.current;
      if (!textarea) return false;
      const target = event.target;
      return (
        target === textarea ||
        (target instanceof Node && textarea.contains(target)) ||
        document.activeElement === textarea
      );
    },
    [editing, textareaRef],
  );
  useClarificationEscapeGuard(editing ? editEscapeGuard : null);
}
type QueuedGhostSaveArgs = {
  value: string;
  entryContent: string;
  entityReferences: readonly EntityReference[];
  attachments?: QueuedMessage["attachments"];
  onSave: QueuedGhostMessageProps["onSave"];
  onEditComplete?: (editToken?: string, saved?: boolean) => void | Promise<void>;
  editTokenRef: React.RefObject<string | undefined>;
  setEditing: (editing: boolean) => void;
  setSaving: (saving: boolean) => void;
  t: (key: string) => string;
};

function useQueuedGhostSave({
  value,
  entryContent,
  entityReferences,
  attachments,
  onSave,
  onEditComplete,
  editTokenRef,
  setEditing,
  setSaving,
  t,
}: QueuedGhostSaveArgs) {
  return useCallback(async () => {
    const editToken = editTokenRef.current;
    const trimmed = value.trim();
    const hasAttachments = (attachments?.length ?? 0) > 0;
    if ((!trimmed && !hasAttachments) || trimmed === entryContent) {
      setEditing(false);
      await onEditComplete?.(editToken, false);
      return;
    }
    setSaving(true);
    try {
      const updatedReferences = survivingEntityReferences(trimmed, entityReferences);
      if (attachments === undefined) {
        await onSave(trimmed, updatedReferences);
      } else {
        await onSave(trimmed, updatedReferences, attachments);
      }
      setEditing(false);
      await onEditComplete?.(editToken, true);
    } catch (err) {
      console.error("Failed to update queued entry:", err);
      if (err instanceof QueueEntryNotFoundError) {
        toast.error(t("chat:queueEditAlreadySent"));
      } else {
        toast.error(t("chat:queueEditSaveFailed"));
      }
      setEditing(false);
      await onEditComplete?.(editToken, false);
    } finally {
      setSaving(false);
    }
  }, [
    value,
    entryContent,
    onSave,
    onEditComplete,
    entityReferences,
    attachments,
    editTokenRef,
    setEditing,
    setSaving,
    t,
  ]);
}

type QueuedGhostCancelArgs = {
  entryContent: string;
  onEditComplete?: (editToken?: string, saved?: boolean) => void | Promise<void>;
  editTokenRef: React.RefObject<string | undefined>;
  setValue: (value: string) => void;
  setEditing: (editing: boolean) => void;
};

function useQueuedGhostCancel({
  entryContent,
  onEditComplete,
  editTokenRef,
  setValue,
  setEditing,
}: QueuedGhostCancelArgs) {
  return useCallback(async () => {
    const editToken = editTokenRef.current;
    setValue(entryContent);
    setEditing(false);
    await onEditComplete?.(editToken);
  }, [editTokenRef, entryContent, onEditComplete, setEditing, setValue]);
}

function useQueuedGhostMetadata(entry: QueuedMessage, canMerge: boolean) {
  const entityReferences = useMemo(
    () => entityReferencesFromMetadata(entry.metadata),
    [entry.metadata],
  );
  return { entityReferences, effectiveCanMerge: canMerge && canMergeEntry(entry) };
}

export const QueuedGhostMessage = forwardRef<QueuedGhostMessageHandle, QueuedGhostMessageProps>(
  function QueuedGhostMessage(
    {
      entry,
      index,
      canEdit,
      editLeaseActive = true,
      canRemove = true,
      canMerge = false,
      canDrag = true,
      showDragHandle = true,
      isDragging = false,
      onSave,
      onRemove,
      onMerge,
      onSendNow = () => undefined,
      sendNowDisabled = false,
      onEditStart,
      onEditComplete,
    },
    ref,
  ) {
    const { t } = useTranslation();
    const [editing, setEditing] = useState(false);
    const [value, setValue] = useState(entry.content);
    const [saving, setSaving] = useState(false);
    const editTokenRef = useRef<string | undefined>(undefined);
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const { entityReferences, effectiveCanMerge } = useQueuedGhostMetadata(entry, canMerge);
    useFocusQueuedEdit(editing, textareaRef);
    useQueuedGhostLeaseLoss(editing, editLeaseActive, entry.content, setValue, setEditing);
    const startEdit = useQueuedGhostStartEdit({
      canEdit,
      editing,
      saving,
      onEditStart,
      onStart: (editToken) => {
        editTokenRef.current = editToken;
        setValue(entry.content);
        setEditing(true);
      },
    });
    useImperativeHandle(ref, () => ({ startEdit }), [startEdit]);
    const handleCancel = useQueuedGhostCancel({
      entryContent: entry.content,
      onEditComplete,
      editTokenRef,
      setValue,
      setEditing,
    });
    const handleSave = useQueuedGhostSave({
      value,
      entryContent: entry.content,
      entityReferences,
      attachments: entry.attachments,
      onSave,
      onEditComplete,
      editTokenRef,
      setEditing,
      setSaving,
      t,
    });
    return (
      <SortableRowShell
        id={entry.id}
        disabled={editing || !canDrag}
        canDrag={canDrag}
        showHandle={showDragHandle && !editing}
        isDragging={isDragging}
      >
        {editing ? (
          <QueuedGhostEditView
            value={value}
            saving={saving}
            attachments={(entry.attachments ?? []) as QueuedAttachment[]}
            onChange={setValue}
            onSave={handleSave}
            onCancel={handleCancel}
            textareaRef={textareaRef}
          />
        ) : (
          <DisplayView
            entry={entry}
            entityReferences={entityReferences}
            positionLabel={queuePositionLabel(index, entry.position)}
            canEdit={canEdit}
            canRemove={canRemove}
            canMerge={effectiveCanMerge}
            onStartEdit={startEdit}
            onRemove={onRemove}
            onMerge={onMerge}
            onSendNow={onSendNow}
            sendNowDisabled={sendNowDisabled}
          />
        )}
      </SortableRowShell>
    );
  },
);
