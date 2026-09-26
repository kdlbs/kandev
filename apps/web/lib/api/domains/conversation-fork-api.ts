import { fetchJson, type ApiRequestOptions } from "../client";

export type ConversationForkEstimate = {
  estimated_tokens: number;
  method: string;
  model_id?: string;
  context_limit?: number | null;
  limit_source?: string;
  attachments_unmeasured: boolean;
};

export type ConversationForkAttachment = {
  source_id?: string;
  id?: string;
  name: string;
  media_type?: string;
  kind?: string;
  delivery_mode?: "prompt" | "path" | string;
  size?: number;
  available: boolean;
};

export type ConversationForkDescriptor = {
  id: string;
  source_task_id: string;
  source_session_id: string;
  source_message_id: string;
  start_message_id?: string;
  source_task_title?: string;
  source_revision: number;
  compiler_version: string;
  content_hash: string;
  message_count: number;
  text_bytes: number;
  omissions: Record<string, number>;
  attachments: ConversationForkAttachment[];
  estimate: ConversationForkEstimate;
  created_at: string;
  expires_at: string;
  state: "draft" | "attached" | "expired" | string;
  destination_kind?: string;
  destination_task_id?: string;
  destination_session_id?: string;
};

export type ConversationForkCandidates = {
  task_id: string;
  session_id: string;
  task_title: string;
  revision: number;
  cutoff_message_id: string;
  cutoff_turn_complete: boolean;
  attachments: ConversationForkAttachment[];
  attachments_has_more: boolean;
  attachment_cursor?: string;
};

export type CreateConversationForkRequest = {
  cutoff_message_id: string;
  start_message_id?: string;
  draft_request_id: string;
  include_tool_evidence: boolean;
  model_id?: string;
  attachment_ids: string[];
};

export type ConversationForkContent = {
  content: string;
  content_hash: string;
  compiler_version: string;
};

const forkCandidatesPath = (sessionId: string) =>
  `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/fork-candidates`;
const createForkPath = (sessionId: string) =>
  `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/conversation-forks`;
const forkPath = (forkId: string) => `/api/v1/conversation-forks/${encodeURIComponent(forkId)}`;

export async function listConversationForkCandidates(
  sessionId: string,
  cutoffMessageId: string,
  params?: { attachmentCursor?: string; startMessageId?: string },
  options?: ApiRequestOptions,
): Promise<ConversationForkCandidates> {
  const query = new URLSearchParams({ cutoff_message_id: cutoffMessageId });
  if (params?.startMessageId) query.set("start_message_id", params.startMessageId);
  if (params?.attachmentCursor) query.set("attachment_cursor", params.attachmentCursor);
  return fetchJson<ConversationForkCandidates>(
    `${forkCandidatesPath(sessionId)}?${query}`,
    options,
  );
}

export async function createConversationForkDraft(
  sessionId: string,
  request: CreateConversationForkRequest,
  options?: ApiRequestOptions,
): Promise<ConversationForkDescriptor> {
  return fetchJson<ConversationForkDescriptor>(createForkPath(sessionId), {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify(request),
      ...(options?.init ?? {}),
    },
  });
}

export async function getConversationForkDraft(
  forkId: string,
  options?: ApiRequestOptions,
): Promise<ConversationForkDescriptor> {
  return fetchJson<ConversationForkDescriptor>(forkPath(forkId), options);
}

export async function getConversationForkContent(
  forkId: string,
  options?: ApiRequestOptions,
): Promise<ConversationForkContent> {
  return fetchJson<ConversationForkContent>(`${forkPath(forkId)}/content`, options);
}

export async function estimateConversationForkDraft(
  forkId: string,
  modelId: string,
  options?: ApiRequestOptions,
): Promise<ConversationForkEstimate> {
  return fetchJson<ConversationForkEstimate>(`${forkPath(forkId)}/estimate`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ model_id: modelId }),
      ...(options?.init ?? {}),
    },
  });
}

export async function discardConversationForkDraft(
  forkId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  return fetchJson<void>(forkPath(forkId), {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}
