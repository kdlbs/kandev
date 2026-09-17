import type { ApiClient } from "./api-client";

type SessionMessage = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];

export type TransientRetryNotice = {
  id: string;
  attempt: number;
  content: string;
};

export async function listTransientRetryNotices(
  apiClient: ApiClient,
  sessionId: string,
): Promise<TransientRetryNotice[]> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter(isTransientRetryNotice).map((message) => ({
    id: message.id,
    attempt: typeof message.metadata?.attempt === "number" ? message.metadata.attempt : 0,
    content: message.content,
  }));
}

function isTransientRetryNotice(message: SessionMessage): boolean {
  return message.type === "status" && message.metadata?.retrying === true;
}
