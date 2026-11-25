"use client";

import { IconFile } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import { formatBytes } from "@/lib/utils/format-bytes";
import { ImageItem } from "@/components/task/chat/context-items/image-item";
import { PREVIEWABLE_IMAGE_TYPES } from "@/components/task/chat/file-attachment";
import type { MessageAttachment } from "@/lib/services/session-launch-service";

export function AttachmentLinks({ attachments }: { attachments?: readonly MessageAttachment[] }) {
  const { t } = useTranslation();
  const linked = attachments?.filter(
    (attachment): attachment is MessageAttachment & { attachment_id: string } =>
      Boolean(attachment.attachment_id),
  );
  if (!linked?.length) return null;
  return (
    <ul data-testid="comment-attachment-links" className="mt-1 flex flex-wrap gap-1 text-xs">
      {linked.map(({ attachment_id: id, type, mime_type, name, size_bytes }) => {
        const fileName = name || t("task:unnamed");
        const image =
          PREVIEWABLE_IMAGE_TYPES.includes(mime_type) || (!mime_type && type === "image");
        return (
          <li key={id} className="min-w-0 max-w-full" onClick={(event) => event.stopPropagation()}>
            {image ? (
              <ImageItem
                showDeliveryMode={false}
                item={{
                  kind: "image",
                  id,
                  label:
                    size_bytes == null
                      ? t("task:image")
                      : t("task:imageWithSize", { bytes: formatBytes(size_bytes) }),
                  attachment: {
                    id,
                    attachmentId: id,
                    fileName,
                    mimeType: mime_type,
                    size: size_bytes ?? 0,
                    isImage: true,
                    deliveryMode: "path",
                    uploadStatus: "ready",
                  },
                }}
              />
            ) : (
              <a
                href={attachmentContentUrl(id)}
                target="_blank"
                rel="noreferrer"
                aria-label={fileName}
                title={fileName}
                className="inline-flex max-w-full items-center gap-1 rounded border border-border/50 bg-muted/50 px-2 py-0.5 text-muted-foreground hover:bg-muted/80 max-md:min-h-11 pointer-coarse:min-h-11"
              >
                <IconFile className="size-3 shrink-0" />
                <span className="max-w-[120px] truncate">{fileName}</span>
                {size_bytes != null && (
                  <span className="shrink-0">({formatBytes(size_bytes)})</span>
                )}
              </a>
            )}
          </li>
        );
      })}
    </ul>
  );
}
