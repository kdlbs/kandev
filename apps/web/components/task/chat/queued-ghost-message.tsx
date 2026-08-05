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
import {
  IconCheck,
  IconChevronDown,
  IconChevronUp,
  IconEdit,
  IconFile,
  IconInfoCircle,
  IconRobot,
  IconSend,
  IconUser,
  IconX,
  IconArrowMerge,
} from "@tabler/icons-react";
import ReactMarkdown from "react-markdown";
import { toast } from "@/lib/toast/sonner";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui";
import { Textarea } from "@kandev/ui/textarea";
import { cn } from "@/lib/utils";
import { QueueEntryNotFoundError } from "@/lib/api/domains/queue-api";
import { stripSystemTags } from "@/lib/utils/system-tags";
import { ImagePreviewDialog } from "@/components/task/chat/image-preview-dialog";
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
import { t } from "@/lib/i18n";

type QueuedAttachment = NonNullable<QueuedMessage["attachments"]>[number];

type AttachmentRowProps = {
  attachments: QueuedAttachment[];
  interactive: boolean;
};

/**
 * Renders queued-message attachments as compact thumbnails (images) and chips
 * (other resources). Used in both display and edit views; `interactive=false`
 * disables the click-to-open behavior so it stays a passive context cue while
 * editing the message text.
 */
function AttachmentRow({ attachments, interactive }: AttachmentRowProps) {
  const { t } = useTranslation();
  if (attachments.length === 0) return null;
  const images = attachments.filter((a) => a.type === "image");
  const files = attachments.filter((a) => a.type !== "image");
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {images.map((att, i) => (
        <ImagePreviewDialog
          key={`img-${i}`}
          src={`data:${att.mime_type};base64,${att.data}`}
          alt={t("task:attachmentIndexed", { index: i + 1 })}
          interactive={interactive}
          thumbnailClassName={cn(
            "h-10 w-10 rounded-md border border-border object-cover",
            interactive && "transition-opacity hover:opacity-90",
          )}
        />
      ))}
      {files.map((_, i) => (
        <span
          key={`file-${i}`}
          className="inline-flex items-center gap-1.5 rounded-full bg-muted/60 px-2 py-0.5 text-xs text-muted-foreground"
        >
          <IconFile className="h-3 w-3" />
          {t("task:attachment")}
        </span>
      ))}
    </div>
  );
}

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

type EditViewProps = {
  value: string;
  saving: boolean;
  attachments: QueuedAttachment[];
  onChange: (v: string) => void;
  onSave: () => void;
  onCancel: () => void;
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
};

function EditView({
  value,
  saving,
  attachments,
  onChange,
  onSave,
  onCancel,
  textareaRef,
}: EditViewProps) {
  const { t } = useTranslation();
  const onKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      onCancel();
    } else if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      onSave();
    }
  };
  return (
    <div className="space-y-2 py-2">
      <AttachmentRow attachments={attachments} interactive={false} />
      <Textarea
        ref={textareaRef}
        data-testid="queue-edit-textarea"
        value={value}
        disabled={saving}
        placeholder={t("task:enterMessageContent")}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
        className={cn(
          "min-h-[60px] max-h-[200px] resize-none overflow-y-auto bg-background border-border",
        )}
      />
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="default"
          onClick={onSave}
          disabled={saving || !value.trim()}
          className="h-7 cursor-pointer"
        >
          <IconCheck className="mr-1 h-3.5 w-3.5" />
          {t("common:save")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={onCancel}
          disabled={saving}
          className="h-7 cursor-pointer"
        >
          {t("common:cancel")}
        </Button>
        <span className="ml-auto text-xs text-muted-foreground">
          {t("task:pressEscToCancelCmdEnter")}
        </span>
      </div>
    </div>
  );
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

/** Rough threshold above which we offer a per-row expand toggle. Two lines of
 * the row's text width fit ~80 chars OR any explicit newline implies overflow,
 * so we use either signal to surface the chevron — short multi-line messages
 * (lists, code blocks) would otherwise be silently truncated. */
const EXPAND_THRESHOLD = 80;

function queuePositionLabel(index: number | undefined, position: number | undefined): string {
  return `#${index === undefined ? (position ?? 1) : index + 1}`;
}

function shouldOfferExpand(text: string): boolean {
  return text.length > EXPAND_THRESHOLD || text.includes("\n");
}

type RowActionsProps = {
  canExpand: boolean;
  expanded: boolean;
  canMerge: boolean;
  canEdit: boolean;
  canRemove: boolean;
  onToggleExpand: () => void;
  onMerge?: () => void | Promise<void>;
  onStartEdit: () => void;
  onRemove: () => void;
  onSendNow?: () => void;
  sendNowDisabled: boolean;
};

function RowActions({
  canExpand,
  expanded,
  canMerge,
  canEdit,
  canRemove,
  onToggleExpand,
  onMerge,
  onStartEdit,
  onRemove,
  onSendNow,
  sendNowDisabled,
}: RowActionsProps) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        "flex items-center gap-0.5 flex-shrink-0 transition-opacity",
        // Hover-reveal on devices that support hover (desktop); always
        // visible on touch surfaces where there's no hover affordance.
        "opacity-0 group-hover:opacity-100 focus-within:opacity-100",
        "[@media(hover:none)]:opacity-100",
        "[@media(pointer:coarse)]:opacity-100",
      )}
    >
      <Button
        variant="ghost"
        size="sm"
        className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
        onClick={onSendNow}
        disabled={sendNowDisabled}
        title={t("chat:sendNowQueuedMessage")}
        data-testid="queue-entry-send-now"
      >
        <IconSend className="h-3.5 w-3.5" />
      </Button>
      {canExpand && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onToggleExpand}
          title={expanded ? t("chat:collapseMessage") : t("chat:expandMessage")}
          data-testid="queue-entry-expand"
          aria-expanded={expanded}
        >
          {expanded ? (
            <IconChevronUp className="h-3.5 w-3.5" />
          ) : (
            <IconChevronDown className="h-3.5 w-3.5" />
          )}
        </Button>
      )}
      {canMerge && onMerge && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onMerge}
          title={t("chat:mergeWithAbove")}
          data-testid="queue-entry-merge"
        >
          <IconArrowMerge className="h-3.5 w-3.5" />
        </Button>
      )}
      {canEdit && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onStartEdit}
          title={t("chat:editQueuedMessage")}
          data-testid="queue-entry-edit"
        >
          <IconEdit className="h-3.5 w-3.5" />
        </Button>
      )}
      {canRemove && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onRemove}
          title={t("chat:removeQueuedMessage")}
          data-testid="queue-entry-remove"
        >
          <IconX className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
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
  const canExpand = shouldOfferExpand(visible);
  const referenceMarkdownComponents = useMemo(
    () => buildEntityReferenceMarkdownComponents(entityReferences),
    [entityReferences],
  );
  return (
    <div className="group flex items-start gap-2 py-1.5" data-testid="queue-entry">
      <span className="flex items-center gap-1.5 mt-0.5 text-muted-foreground">
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
            data-testid="queue-entry-text"
            data-expanded={expanded ? "true" : "false"}
            className={cn(
              "markdown-body max-w-none text-sm text-foreground/80 break-words overflow-hidden",
              "[&>*:first-child]:mt-0 [&>*:last-child]:mb-0",
              "transition-[max-height] duration-200 ease-out motion-reduce:transition-none",
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
      <RowActions
        canExpand={canExpand}
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

type QueuedGhostMessageProps = {
  entry: QueuedMessage;
  /** Zero-based render position; combined with entry.position to label the row. */
  index?: number;
  /**
   * Edit is only allowed when the caller's identity (currentUserId) matches the
   * entry's queued_by. Inter-task entries are visible but read-only.
   */
  canEdit: boolean;
  /** Removal is independent from edit ownership and applies to every visible row. */
  canRemove?: boolean;
  /**
   * Merge-with-above control; only shown when this row and the one above it
   * share a sender kind. Optional so standalone renders (tests) can omit it.
   */
  canMerge?: boolean;
  onSave: (content: string, entityReferences: EntityReference[]) => Promise<void>;
  onRemove: () => void | Promise<void>;
  /** Fold this entry into the one above it. */
  onMerge?: () => void | Promise<void>;
  /** Interrupt the current turn and dispatch this exact queued entry. */
  onSendNow?: () => void;
  /** Disable while the queue mutation or backend cancellation is in flight. */
  sendNowDisabled?: boolean;
  /** Called after edit save/cancel so the parent can refocus the chat input. */
  onEditComplete?: () => void;
};

function useFocusQueuedEdit(
  editing: boolean,
  textareaRef: React.RefObject<HTMLTextAreaElement | null>,
): void {
  useEffect(() => {
    if (!editing || !textareaRef.current) return;
    const textarea = textareaRef.current;
    textarea.focus();
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
  }, [editing, textareaRef]);
}

type QueuedGhostSaveArgs = {
  value: string;
  entryContent: string;
  entityReferences: readonly EntityReference[];
  onSave: QueuedGhostMessageProps["onSave"];
  onEditComplete?: () => void;
  setEditing: (editing: boolean) => void;
  setSaving: (saving: boolean) => void;
  t: (key: string) => string;
};

function useQueuedGhostSave({
  value,
  entryContent,
  entityReferences,
  onSave,
  onEditComplete,
  setEditing,
  setSaving,
  t,
}: QueuedGhostSaveArgs) {
  return useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || trimmed === entryContent) {
      setEditing(false);
      onEditComplete?.();
      return;
    }
    setSaving(true);
    try {
      await onSave(trimmed, survivingEntityReferences(trimmed, entityReferences));
      setEditing(false);
      onEditComplete?.();
    } catch (err) {
      console.error("Failed to update queued entry:", err);
      if (err instanceof QueueEntryNotFoundError) {
        toast.error(t("chat:queueEditAlreadySent"));
      } else {
        toast.error(t("chat:queueEditSaveFailed"));
      }
      setEditing(false);
      onEditComplete?.();
    } finally {
      setSaving(false);
    }
  }, [value, entryContent, onSave, onEditComplete, entityReferences, setEditing, setSaving, t]);
}

export const QueuedGhostMessage = forwardRef<QueuedGhostMessageHandle, QueuedGhostMessageProps>(
  function QueuedGhostMessage(
    {
      entry,
      index,
      canEdit,
      canRemove = true,
      canMerge = false,
      onSave,
      onRemove,
      onMerge,
      onSendNow = () => undefined,
      sendNowDisabled = false,
      onEditComplete,
    },
    ref,
  ) {
    const { t } = useTranslation();
    const [editing, setEditing] = useState(false);
    const [value, setValue] = useState(entry.content);
    const [saving, setSaving] = useState(false);
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const entityReferences = useMemo(
      () => entityReferencesFromMetadata(entry.metadata),
      [entry.metadata],
    );
    const effectiveCanMerge = canMerge && canMergeEntry(entry);
    useFocusQueuedEdit(editing, textareaRef);
    useEffect(() => {
      if (!editing) setValue(entry.content);
    }, [entry.content, editing]);

    const startEdit = useCallback(() => {
      if (!canEdit) return;
      setValue(entry.content);
      setEditing(true);
    }, [entry.content, canEdit]);

    useImperativeHandle(ref, () => ({ startEdit }), [startEdit]);
    const handleCancel = useCallback(() => {
      setValue(entry.content);
      setEditing(false);
      onEditComplete?.();
    }, [entry.content, onEditComplete]);

    const handleSave = useQueuedGhostSave({
      value,
      entryContent: entry.content,
      entityReferences,
      onSave,
      onEditComplete,
      setEditing,
      setSaving,
      t,
    });

    const positionLabel = queuePositionLabel(index, entry.position);

    return (
      <div
        className={cn(
          "rounded-md border border-border/60 bg-background/40 px-2 text-sm",
          "hover:border-border transition-colors",
        )}
      >
        {editing ? (
          <EditView
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
            positionLabel={positionLabel}
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
      </div>
    );
  },
);
