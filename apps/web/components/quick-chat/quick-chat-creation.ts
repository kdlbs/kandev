import type { MessageAttachment } from "@/lib/services/session-launch-service";
import { generateUUID } from "@/lib/utils";
import type { QuickChatRepositoryInput } from "@/lib/api/domains/workspace-api";

export type QuickChatCreationState = "submitting" | "unknown" | "incomplete" | "accepted";

export type PendingQuickChatCreation = {
  creationId: string;
  clientMessageId: string;
  message: string;
  attachments: MessageAttachment[];
  agentProfileId: string;
  repositories: QuickChatRepositoryInput[];
  runtimeId: string;
  state: QuickChatCreationState;
  messageAttempted: boolean;
  taskId?: string;
  sessionId?: string;
  sessionState?: string;
  archived?: boolean;
};

export const QUICK_CHAT_RUNTIME_ID = generateUUID();

/** Keep the retry receipt useful while excluding inline file contents. */
export function toQuickChatAttachmentDescriptors(
  attachments: MessageAttachment[] | undefined,
): MessageAttachment[] {
  return (attachments ?? []).map(({ data: _data, ...descriptor }) => descriptor);
}

export function pendingCreationPayload(creation: PendingQuickChatCreation) {
  return {
    message: creation.message,
    clientMessageId: creation.clientMessageId,
    ...(creation.attachments.length ? { attachments: creation.attachments } : {}),
  };
}

export function isCreationStatusResponse(
  value: unknown,
  creationId: string,
): value is {
  task_id: string;
  creation_id: string;
  creation_complete: boolean;
  session_id?: string;
  agent_profile_id?: string;
  session_state?: string;
  archived?: boolean;
} {
  if (!value || typeof value !== "object") return false;
  const response = value as Record<string, unknown>;
  return (
    response.creation_id === creationId &&
    typeof response.task_id === "string" &&
    response.task_id.length > 0 &&
    typeof response.creation_complete === "boolean" &&
    (response.session_id === undefined || typeof response.session_id === "string")
  );
}
