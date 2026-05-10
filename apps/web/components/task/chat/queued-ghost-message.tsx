"use client";

import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import { IconCheck, IconClock, IconEdit, IconRobot, IconUser, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

/** Strip internal <kandev-system>...</kandev-system> blocks from display text. */
function stripSystemTags(text: string): string {
  return text.replace(/<kandev-system>[\s\S]*?<\/kandev-system>/g, "").trim();
}

/** Imperative handle for the ghost row, used by chat input "edit last queued" affordance. */
export type QueuedGhostMessageHandle = {
  startEdit: () => void;
};

type SenderKind = "user" | "agent" | "system";

function senderKindOf(entry: QueuedMessage): SenderKind {
  if (!entry.queued_by) return "system";
  if (entry.queued_by === "agent") return "agent";
  // Inter-task messages carry sender_task_id in metadata even though queued_by is "agent".
  if (entry.metadata && (entry.metadata.sender_task_id || entry.metadata.sender_task_title)) {
    return "agent";
  }
  return "user";
}

function senderLabel(entry: QueuedMessage): string {
  const kind = senderKindOf(entry);
  if (kind === "agent") {
    const title = entry.metadata?.sender_task_title;
    return typeof title === "string" && title.length > 0 ? `From ${title}` : "From agent";
  }
  if (kind === "system") return "System";
  return "You";
}

type SenderChipProps = { entry: QueuedMessage };

function SenderChip({ entry }: SenderChipProps) {
  const kind = senderKindOf(entry);
  const Icon = kind === "agent" ? IconRobot : IconUser;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide",
        kind === "agent"
          ? "bg-amber-500/15 text-amber-600 dark:text-amber-400"
          : "bg-primary/10 text-primary",
      )}
    >
      <Icon className="h-3 w-3" />
      {senderLabel(entry)}
    </span>
  );
}

type EditViewProps = {
  value: string;
  saving: boolean;
  onChange: (v: string) => void;
  onSave: () => void;
  onCancel: () => void;
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
};

function EditView({ value, saving, onChange, onSave, onCancel, textareaRef }: EditViewProps) {
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
    <div className="space-y-2 p-2">
      <Textarea
        ref={textareaRef}
        data-testid="queue-edit-textarea"
        value={value}
        disabled={saving}
        placeholder="Enter message content..."
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
          Save
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={onCancel}
          disabled={saving}
          className="h-7 cursor-pointer"
        >
          Cancel
        </Button>
        <span className="ml-auto text-xs text-muted-foreground">
          Press Esc to cancel, Cmd+Enter to save
        </span>
      </div>
    </div>
  );
}

type DisplayViewProps = {
  entry: QueuedMessage;
  canEdit: boolean;
  onStartEdit: () => void;
  onRemove: () => void;
};

function DisplayView({ entry, canEdit, onStartEdit, onRemove }: DisplayViewProps) {
  const visible = stripSystemTags(entry.content);
  const display = visible.length > 200 ? visible.slice(0, 200) + "..." : visible;
  return (
    <div className="flex items-start gap-2 px-3 py-2">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="mt-0.5 flex items-center gap-1 text-muted-foreground">
            <IconClock className="h-3.5 w-3.5" />
          </span>
        </TooltipTrigger>
        <TooltipContent side="top">Will run on the next turn</TooltipContent>
      </Tooltip>
      <div className="flex-1 min-w-0 space-y-1">
        <SenderChip entry={entry} />
        <div className="text-sm text-foreground/80 whitespace-pre-wrap break-words">{display}</div>
      </div>
      <div className="flex items-center gap-0.5 flex-shrink-0">
        {canEdit && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground"
            onClick={onStartEdit}
            title="Edit queued message"
          >
            <IconEdit className="h-3.5 w-3.5" />
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground"
          onClick={onRemove}
          title="Remove queued message"
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

type QueuedGhostMessageProps = {
  entry: QueuedMessage;
  /**
   * Edit is only allowed when the caller's identity (currentUserId) matches the
   * entry's queued_by. Inter-task entries are visible but read-only.
   */
  canEdit: boolean;
  onSave: (content: string) => Promise<void>;
  onRemove: () => void | Promise<void>;
  /** Called after edit save/cancel so the parent can refocus the chat input. */
  onEditComplete?: () => void;
};

export const QueuedGhostMessage = forwardRef<QueuedGhostMessageHandle, QueuedGhostMessageProps>(
  function QueuedGhostMessage({ entry, canEdit, onSave, onRemove, onEditComplete }, ref) {
    const [editing, setEditing] = useState(false);
    const [value, setValue] = useState(entry.content);
    const [saving, setSaving] = useState(false);
    const textareaRef = useRef<HTMLTextAreaElement>(null);

    useEffect(() => {
      if (!editing) setValue(entry.content);
    }, [entry.content, editing]);

    useEffect(() => {
      if (editing && textareaRef.current) {
        const el = textareaRef.current;
        el.focus();
        el.setSelectionRange(el.value.length, el.value.length);
      }
    }, [editing]);

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

    const handleSave = useCallback(async () => {
      const trimmed = value.trim();
      if (!trimmed || trimmed === entry.content) {
        setEditing(false);
        onEditComplete?.();
        return;
      }
      setSaving(true);
      try {
        await onSave(trimmed);
        setEditing(false);
        onEditComplete?.();
      } catch (err) {
        console.error("Failed to update queued entry:", err);
      } finally {
        setSaving(false);
      }
    }, [value, entry.content, onSave, onEditComplete]);

    return (
      <div
        className={cn(
          "rounded-md border-l-2 bg-muted/40 text-sm",
          senderKindOf(entry) === "agent" ? "border-amber-400/60" : "border-primary/40",
        )}
      >
        {editing ? (
          <EditView
            value={value}
            saving={saving}
            onChange={setValue}
            onSave={handleSave}
            onCancel={handleCancel}
            textareaRef={textareaRef}
          />
        ) : (
          <DisplayView
            entry={entry}
            canEdit={canEdit}
            onStartEdit={startEdit}
            onRemove={onRemove}
          />
        )}
      </div>
    );
  },
);
