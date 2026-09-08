import type { QueueMessageParams } from "@/lib/api/domains/queue-api";
import type { EntityReference } from "@/lib/types/entity-reference";

export type MessageAttachment = {
  type: string;
  data?: string;
  attachment_id?: string;
  mime_type: string;
  name?: string;
  size_bytes?: number;
  delivery_mode?: "prompt" | "path";
};

export type QueueMessageInput = {
  taskId: string;
  content: string;
  model?: string;
  planMode?: boolean;
  attachments?: MessageAttachment[];
  entityReferences?: EntityReference[];
  contextFilesMeta?: QueueMessageParams["context_files"];
};
