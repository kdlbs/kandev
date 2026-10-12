"use client";

import type { KeyboardEvent, RefObject } from "react";
import { IconCheck } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui";
import { Textarea } from "@kandev/ui/textarea";
import { cn } from "@/lib/utils";
import { AttachmentRow, type QueuedAttachment } from "@/components/task/chat/queued-attachment-row";

type QueuedGhostEditViewProps = {
  value: string;
  saving: boolean;
  attachments: QueuedAttachment[];
  onChange: (value: string) => void;
  onSave: () => void;
  onCancel: () => void;
  textareaRef: RefObject<HTMLTextAreaElement | null>;
};

export function QueuedGhostEditView({
  value,
  saving,
  attachments,
  onChange,
  onSave,
  onCancel,
  textareaRef,
}: QueuedGhostEditViewProps) {
  const { t } = useTranslation();
  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      // Claim the key here: once this edit is cancelled, nothing further up
      // the tree (e.g. a clarification panel's own Escape-collapses handler)
      // should also react to the same keypress.
      event.stopPropagation();
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
        onChange={(event) => onChange(event.target.value)}
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
          disabled={saving || (!value.trim() && attachments.length === 0)}
          className="h-7 cursor-pointer [@media(pointer:coarse)]:h-11"
        >
          <IconCheck className="mr-1 h-3.5 w-3.5" />
          {t("common:save")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={onCancel}
          disabled={saving}
          className="h-7 cursor-pointer [@media(pointer:coarse)]:h-11"
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
