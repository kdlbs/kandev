import type { MessageAttachment } from "@/lib/services/session-launch-service";

/** Convert the opening payload's wire descriptors into the local composer draft shape. */
export function toQuickChatDraftAttachments(attachments?: MessageAttachment[]) {
  return (
    attachments?.flatMap((attachment) => {
      if (!attachment.attachment_id || !attachment.name) return [];
      return [
        {
          id: attachment.attachment_id,
          attachmentId: attachment.attachment_id,
          mimeType: attachment.mime_type,
          fileName: attachment.name,
          size: attachment.size_bytes ?? 0,
          isImage: attachment.type === "image",
          deliveryMode:
            attachment.delivery_mode ?? (attachment.type === "image" ? "prompt" : "path"),
        },
      ];
    }) ?? []
  );
}
