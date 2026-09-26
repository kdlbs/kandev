"use client";

import { IconFile } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import { ImagePreviewDialog } from "@/components/task/chat/image-preview-dialog";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

export type QueuedAttachment = NonNullable<QueuedMessage["attachments"]>[number];

type AttachmentRowProps = {
  attachments: QueuedAttachment[];
  interactive: boolean;
};

function queuedAttachmentSource(attachment: QueuedAttachment): string | null {
  if (attachment.attachment_id) return attachmentContentUrl(attachment.attachment_id);
  if (attachment.data) return `data:${attachment.mime_type};base64,${attachment.data}`;
  return null;
}

/**
 * Renders queued-message attachments as compact thumbnails (images) and chips
 * (other resources). Used in both display and edit views; `interactive=false`
 * disables the click-to-open behavior so it stays a passive context cue while
 * editing the message text.
 */
export function AttachmentRow({ attachments, interactive }: AttachmentRowProps) {
  const { t } = useTranslation();
  if (attachments.length === 0) return null;
  const images = attachments.filter((a) => a.type === "image");
  const files = attachments.filter((a) => a.type !== "image");
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {images.map((att, i) => {
        const src = queuedAttachmentSource(att);
        if (!src) {
          return (
            <span
              key={`img-${i}`}
              className="inline-flex items-center gap-1.5 rounded-full bg-muted/60 px-2 py-0.5 text-xs text-muted-foreground"
            >
              <IconFile className="h-3 w-3" />
              {att.name || t("task:attachment")}
            </span>
          );
        }
        return (
          <ImagePreviewDialog
            key={`img-${i}`}
            src={src}
            alt={t("task:attachmentIndexed", { index: i + 1 })}
            interactive={interactive}
            thumbnailClassName={cn(
              "h-10 w-10 rounded-md border border-border object-cover",
              interactive && "transition-opacity hover:opacity-90",
            )}
          />
        );
      })}
      {files.map((att, i) => (
        <span
          key={`file-${i}`}
          className="inline-flex items-center gap-1.5 rounded-full bg-muted/60 px-2 py-0.5 text-xs text-muted-foreground"
        >
          <IconFile className="h-3 w-3" />
          {att.name || t("task:attachment")}
        </span>
      ))}
    </div>
  );
}
