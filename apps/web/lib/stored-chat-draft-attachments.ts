export type StoredFileAttachment = {
  id: string;
  attachmentId?: string;
  expiresAt?: string;
  /** Legacy inline data is read for backwards compatibility only. */
  data?: string;
  mimeType: string;
  fileName: string;
  size: number;
  isImage: boolean;
  deliveryMode?: "prompt" | "path";
};

/** Keep staged descriptors in session storage, but never persist an in-flight File object. */
export function toStoredChatDraftAttachments(
  attachments: Array<StoredFileAttachment & { file?: File; preview?: string }>,
): StoredFileAttachment[] {
  return attachments.flatMap(
    ({
      id,
      file,
      attachmentId,
      expiresAt,
      data,
      mimeType,
      fileName,
      size,
      isImage,
      deliveryMode,
    }) => {
      if ((file && !attachmentId) || (!attachmentId && !data)) return [];
      return [
        {
          id,
          ...(attachmentId ? { attachmentId } : { data }),
          ...(expiresAt ? { expiresAt } : {}),
          mimeType,
          fileName,
          size,
          isImage,
          deliveryMode,
        },
      ];
    },
  );
}
