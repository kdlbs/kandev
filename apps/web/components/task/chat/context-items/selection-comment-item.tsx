"use client";

import { AttachmentLinks } from "@/components/shared/attachment-links";
import { memo } from "react";
import type { MessageAttachment } from "@/lib/services/session-launch-service";
import { ContextChip } from "./context-chip";

type SelectionComment = {
  id: string;
  selectedText: string;
  text: string;
  attachments?: MessageAttachment[];
};

export const SelectionCommentItem = memo(function SelectionCommentItem({
  kind,
  label,
  comments,
  onClick,
  onRemove,
}: {
  kind: "plan-comment" | "agent-message-comment";
  label: string;
  comments: SelectionComment[];
  onClick?: () => void;
  onRemove?: () => void;
}) {
  const preview = (
    <div className="space-y-1.5">
      {comments.map((comment) => (
        <div key={comment.id} className="space-y-0.5 text-xs">
          {comment.selectedText ? (
            <div className="line-clamp-2 text-muted-foreground italic">
              “{comment.selectedText}”
            </div>
          ) : null}
          <div className="break-words">{comment.text}</div>
          <AttachmentLinks attachments={comment.attachments} />
        </div>
      ))}
    </div>
  );

  return (
    <ContextChip
      kind={kind}
      label={label}
      preview={preview}
      onClick={onClick}
      onRemove={onRemove}
    />
  );
});
