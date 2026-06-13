import type { Message, Turn } from "@/lib/types/http";

export type MessageDebugEntries = {
  message_id: string;
  turn_id: string | null;
  type: string;
  model: unknown;
  usage_multiplier: unknown;
  agent_id: unknown;
  agent_type: unknown;
  prompt_usage: unknown;
  message_metadata: Record<string, unknown> | null;
  turn_metadata: Record<string, unknown> | null;
};

export type MessageDebugContext = {
  usageMultiplier?: string | null;
};

function hasOwnMetadata(metadata: Record<string, unknown> | null | undefined): boolean {
  return !!metadata && Object.keys(metadata).length > 0;
}

function firstPresent(...values: unknown[]): unknown {
  return values.find((value) => value != null) ?? null;
}

function metadataValue(metadata: Record<string, unknown> | null, key: string): unknown {
  if (!metadata) return null;
  return metadata[key] ?? null;
}

function resolveTurnID(message: Message, turn: Turn | null | undefined): string | null {
  if (message.turn_id) return message.turn_id;
  return turn?.id ?? null;
}

export function hasMessageDebugMetadata(message: Message, turn?: Turn | null): boolean {
  return hasOwnMetadata(message.metadata) || hasOwnMetadata(turn?.metadata);
}

export function buildMessageDebugEntries(
  message: Message,
  turn?: Turn | null,
  context: MessageDebugContext = {},
): MessageDebugEntries {
  const messageMetadata = message.metadata ?? null;
  const turnMetadata = turn?.metadata ?? null;
  return {
    message_id: message.id,
    turn_id: resolveTurnID(message, turn),
    type: message.type,
    model: firstPresent(
      metadataValue(messageMetadata, "model"),
      metadataValue(turnMetadata, "model"),
    ),
    usage_multiplier: firstPresent(
      metadataValue(messageMetadata, "usage_multiplier"),
      metadataValue(turnMetadata, "usage_multiplier"),
      context.usageMultiplier,
    ),
    agent_id: firstPresent(
      metadataValue(messageMetadata, "agent_id"),
      metadataValue(turnMetadata, "agent_id"),
    ),
    agent_type: firstPresent(
      metadataValue(messageMetadata, "agent_type"),
      metadataValue(turnMetadata, "agent_type"),
    ),
    prompt_usage: firstPresent(
      metadataValue(turnMetadata, "prompt_usage"),
      metadataValue(messageMetadata, "prompt_usage"),
    ),
    message_metadata: messageMetadata,
    turn_metadata: turnMetadata,
  };
}
