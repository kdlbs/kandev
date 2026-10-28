"use client";

import { useCallback } from "react";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import { toMessageAttachments } from "@/components/task-create-dialog-helpers";
import type { TaskCreationReceipt } from "@/lib/task-creation-receipt";

type ComparableAttachment = {
  type?: string;
  attachment_id?: string;
  name?: string;
  mime_type: string;
  size_bytes?: number;
  data?: string;
  delivery_mode?: string;
};

function attachmentSignature(attachments?: ComparableAttachment[]): string {
  return (attachments ?? [])
    .map((attachment) =>
      [
        attachment.attachment_id ?? "inline",
        attachment.name ?? "",
        attachment.mime_type,
        String(attachment.size_bytes ?? 0),
        attachment.data ? "data" : "descriptor",
        attachment.delivery_mode ?? "",
      ].join("\u0000"),
    )
    .join("\u0001");
}

export function useLiveDraftMatch(args: {
  taskNameRef: { current: string };
  descriptionInputRef: React.RefObject<TaskFormInputsHandle | null>;
}) {
  const { taskNameRef, descriptionInputRef } = args;
  return useCallback(
    (candidate: TaskCreationReceipt) => {
      const submittedTitle =
        candidate.draftTitle ??
        (typeof (candidate.payload as { title?: unknown }).title === "string"
          ? ((candidate.payload as { title: string }).title ?? "").trim()
          : "");
      if (taskNameRef.current.trim() !== submittedTitle.trim()) return false;
      const currentDescription = (descriptionInputRef.current?.getValue() ?? "").trim();
      if (currentDescription !== (candidate.payload.description ?? "").trim()) return false;
      const currentAttachments = toMessageAttachments(
        descriptionInputRef.current?.getAttachments() ?? [],
      );
      return (
        attachmentSignature(currentAttachments) ===
        attachmentSignature(candidate.payload.attachments)
      );
    },
    [descriptionInputRef, taskNameRef],
  );
}
