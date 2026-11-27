import type { FileAttachment } from "./file-attachment";
import type { MessageAttachment } from "./chat-input-container";

export function matchesSubmittedAttachments(
  current: FileAttachment[],
  submitted: MessageAttachment[] = [],
): boolean {
  return (
    current.length === submitted.length &&
    current.every((attachment, index) => {
      const expected = submitted[index];
      if (!expected) return false;
      return (
        attachment.attachmentId === expected.attachment_id &&
        attachment.fileName === expected.name &&
        attachment.mimeType === expected.mime_type &&
        attachment.size === expected.size_bytes &&
        attachment.isImage === (expected.type === "image") &&
        attachment.deliveryMode ===
          (expected.delivery_mode ?? (expected.type === "image" ? "prompt" : "path"))
      );
    })
  );
}
