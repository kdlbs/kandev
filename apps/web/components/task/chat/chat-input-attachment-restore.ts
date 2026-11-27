import { getChatDraftAttachments, restoreAttachmentPreview } from "@/lib/local-storage";
import { t } from "@/lib/i18n";
import type { MessageAttachment } from "./chat-input-container";
import { MAX_FILE_SIZE, type FileAttachment } from "./file-attachment";

export function attachmentUploadFailedMessage(): string {
  return t("task:attachmentUploadFailed");
}

export function restoreDraftAttachment(
  attachment: ReturnType<typeof getChatDraftAttachments>[number],
) {
  const restored = restoreAttachmentPreview(attachment);
  if (restored.attachmentId) {
    if (restored.expiresAt && Date.parse(restored.expiresAt) <= Date.now()) {
      return {
        ...restored,
        uploadStatus: "failed" as const,
        uploadError: attachmentUploadFailedMessage(),
      };
    }
    return { ...restored, uploadStatus: "ready" as const };
  }
  if (!restored.data) {
    return {
      ...restored,
      uploadStatus: "failed" as const,
      uploadError: attachmentUploadFailedMessage(),
    };
  }

  try {
    const binary = atob(restored.data);
    if (binary.length !== restored.size || binary.length > MAX_FILE_SIZE) throw new Error();
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
    return {
      ...restored,
      file: new File([bytes], restored.fileName, { type: restored.mimeType }),
      uploadStatus: "pending" as const,
    };
  } catch {
    return {
      ...restored,
      uploadStatus: "failed" as const,
      uploadError: attachmentUploadFailedMessage(),
    };
  }
}

export function restoreOpeningMessageAttachments(
  sessionId: string,
  messageAttachments: MessageAttachment[],
): FileAttachment[] {
  const storedAttachments = getChatDraftAttachments(sessionId);
  return messageAttachments.flatMap((attachment) => {
    if (!attachment.attachment_id || !attachment.name) return [];
    const stored = storedAttachments.find((item) => item.attachmentId === attachment.attachment_id);
    if (stored) return [restoreDraftAttachment(stored)];
    return [
      {
        id: attachment.attachment_id,
        attachmentId: attachment.attachment_id,
        mimeType: attachment.mime_type,
        fileName: attachment.name,
        size: attachment.size_bytes ?? 0,
        isImage: attachment.type === "image",
        deliveryMode: attachment.delivery_mode ?? (attachment.type === "image" ? "prompt" : "path"),
        uploadStatus: "ready" as const,
      },
    ];
  });
}
