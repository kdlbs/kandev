import { toMessageAttachments } from "@/components/task-create-dialog-helpers";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import type { QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";
import type { QuickChatOpeningPayload, QuickChatSessionKind } from "@/lib/state/slices/ui/types";
import { generateUUID } from "@/lib/utils";

export function hasUnavailableAttachment(attachments: FileAttachment[]): boolean {
  return attachments.some(
    (attachment) =>
      attachment.uploadStatus === "uploading" ||
      attachment.uploadStatus === "failed" ||
      (attachment.file && !attachment.attachmentId) ||
      (!attachment.attachmentId && !attachment.data),
  );
}

export function isInvalidOpeningRequest(args: {
  message: string;
  attachments: FileAttachment[];
  repositories: TaskRepoRow[];
  kind: QuickChatSessionKind;
  isStarting: boolean;
  profileEnabled: boolean;
}): boolean {
  return (
    !args.message.trim() ||
    args.isStarting ||
    !args.profileEnabled ||
    hasUnavailableAttachment(args.attachments) ||
    (args.kind === "chat" && args.repositories.some((row) => !row.repositoryId || !row.branch))
  );
}

export function buildOpeningPayload(
  message: string,
  attachments: FileAttachment[],
): QuickChatOpeningPayload {
  const attachmentPayload = toMessageAttachments(attachments) ?? [];
  return {
    message,
    clientMessageId: generateUUID(),
    ...(attachmentPayload.length > 0 ? { attachments: attachmentPayload } : {}),
  };
}

export function toQuickChatRepositoryInputs(rows: TaskRepoRow[]): QuickChatRepositoryInput[] {
  return rows
    .filter((row) => row.repositoryId && row.branch)
    .map((row) => ({ repository_id: row.repositoryId as string, base_branch: row.branch }));
}
