"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { AttachmentPicker } from "@/components/task/chat/attachment-picker";
import {
  PREVIEWABLE_IMAGE_TYPES,
  type FileAttachment,
} from "@/components/task/chat/file-attachment";
import type { useCommentAttachments } from "./use-comment-attachments";

export type CommentAttachments = ReturnType<typeof useCommentAttachments>;

export function CommentAttachmentControl({
  controller,
  disabled,
  children,
}: {
  controller: CommentAttachments;
  disabled?: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const files = controller.files.map((file): FileAttachment => {
    const mimeType = file.descriptor?.mime_type ?? file.local?.type ?? "";
    return {
      id: file.key,
      fileName: file.name,
      file: file.local,
      attachmentId: file.descriptor?.attachment_id,
      uploadStatus: file.status,
      mimeType,
      size: file.descriptor?.size_bytes ?? file.local?.size ?? 0,
      isImage: PREVIEWABLE_IMAGE_TYPES.includes(mimeType),
      deliveryMode: "path",
    };
  });

  return (
    <div className="flex flex-wrap items-center justify-end gap-x-1">
      <div className="min-w-fit flex-1">{children}</div>
      <AttachmentPicker
        files={files}
        onAddFiles={controller.addFiles}
        onRetry={controller.retry}
        onRemove={controller.remove}
        title={t("task:commentAttachments")}
        hint={t("task:commentAttachmentsHint")}
        testId="comment-attachment"
        linkFiles
        disabled={disabled}
      />
    </div>
  );
}
