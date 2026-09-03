import type { StateCreator } from "zustand";
import type { Message } from "@/lib/types/http";
import type { SessionSlice, SessionSliceState } from "./types";
import {
  compareMessageTimestamps,
  isIncomingMessageAtLeastAsFresh,
  messageTimestampNanoseconds,
} from "./message-timestamp";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

type PromptMeta = SessionSliceState["messagePrompts"]["metaBySession"][string];

/** Ensures prompt loading metadata exists for a session. */
function ensurePromptMeta(
  metaBySession: SessionSliceState["messagePrompts"]["metaBySession"],
  sessionId: string,
) {
  if (!metaBySession[sessionId]) {
    metaBySession[sessionId] = {
      isLoading: false,
      isLoadingMore: false,
      historyInitialized: false,
      hasMore: false,
      oldestCursor: null,
    };
  }
}

/** Applies partial loading metadata to a prompt session. */
function applyPromptMeta(
  metaBySession: SessionSliceState["messagePrompts"]["metaBySession"],
  sessionId: string,
  meta: Partial<PromptMeta>,
) {
  ensurePromptMeta(metaBySession, sessionId);
  if (meta.historyInitialized !== undefined) {
    metaBySession[sessionId].historyInitialized = meta.historyInitialized;
  }
  if (meta.hasMore !== undefined) metaBySession[sessionId].hasMore = meta.hasMore;
  if (meta.isLoading !== undefined) metaBySession[sessionId].isLoading = meta.isLoading;
  if (meta.isLoadingMore !== undefined) {
    metaBySession[sessionId].isLoadingMore = meta.isLoadingMore;
  }
  if (meta.oldestCursor !== undefined) {
    metaBySession[sessionId].oldestCursor = meta.oldestCursor;
  }
}

/** Returns whether a row is a valid user prompt with a parseable timestamp. */
function isValidPrompt(message: Message) {
  return message.author_type === "user" && messageTimestampNanoseconds(message.created_at) !== null;
}

/** Orders prompt IDs for stable ties on identical timestamps. */
function comparePromptIDs(left: Message, right: Message) {
  if (left.id < right.id) return -1;
  if (left.id > right.id) return 1;
  return 0;
}

/** Filters invalid rows and sorts prompts by creation order. */
function sortPromptMessages(messages: Message[]) {
  return messages.filter(isValidPrompt).sort((left, right) => {
    const timeDelta = compareMessageTimestamps(left.created_at, right.created_at);
    return timeDelta !== null && timeDelta !== 0 ? timeDelta : comparePromptIDs(left, right);
  });
}

/** Reports whether an incoming prompt update is at least as fresh as cached data. */
function isIncomingPromptAtLeastAsFresh(existing: Message, incoming: Message) {
  return isIncomingMessageAtLeastAsFresh(existing, incoming);
}

/** Merges a prompt update without regressing its immutable creation order. */
function mergePromptMessage(existing: Message, incoming: Message) {
  return isIncomingPromptAtLeastAsFresh(existing, incoming)
    ? { ...existing, ...incoming, created_at: existing.created_at }
    : existing;
}

/** Inserts or refreshes one prompt in the independent prompt cache. */
function upsertPromptMessage(state: SessionSliceState, message: Message) {
  if (!isValidPrompt(message)) return;
  const sessionId = message.session_id;
  const prompts = state.messagePrompts.bySession[sessionId] ?? [];
  const index = prompts.findIndex((prompt) => prompt.id === message.id);
  if (index === -1) prompts.push(message);
  else prompts[index] = mergePromptMessage(prompts[index], message);
  state.messagePrompts.bySession[sessionId] = sortPromptMessages(prompts);
  ensurePromptMeta(state.messagePrompts.metaBySession, sessionId);
}

/** Applies a live user-message update to the prompt cache when present. */
export function updatePromptMessage(state: SessionSliceState, message: Message) {
  if (!isValidPrompt(message)) return;
  const prompts = state.messagePrompts.bySession[message.session_id];
  if (!prompts) return;
  const index = prompts.findIndex((entry) => entry.id === message.id);
  if (index === -1) return;
  prompts[index] = mergePromptMessage(prompts[index], message);
  state.messagePrompts.bySession[message.session_id] = sortPromptMessages(prompts);
}

/** Fans transcript message events into the prompt cache. */
export function fanOutTranscriptPrompts(state: SessionSliceState, messages: Message[]) {
  for (const message of messages) upsertPromptMessage(state, message);
}

/** Removes a prompt and repairs the cached oldest cursor. */
export function removePromptMessage(
  state: SessionSliceState,
  sessionId: string,
  messageId: string,
) {
  const prompts = state.messagePrompts.bySession[sessionId];
  if (!prompts) return;
  state.messagePrompts.bySession[sessionId] = prompts.filter((message) => message.id !== messageId);
  const meta = state.messagePrompts.metaBySession[sessionId];
  if (meta?.oldestCursor === messageId) {
    meta.oldestCursor = state.messagePrompts.bySession[sessionId][0]?.id ?? null;
  }
}

/** Builds the prompt-cache actions consumed by the session slice. */
export function buildPromptMessageActions(set: ImmerSet) {
  return {
    replacePromptMessages: (
      sessionId: string,
      messages: Parameters<SessionSlice["replacePromptMessages"]>[1],
      meta?: Parameters<SessionSlice["replacePromptMessages"]>[2],
    ) =>
      set((draft) => {
        const existingByID = new Map(
          (draft.messagePrompts.bySession[sessionId] ?? []).map((message) => [message.id, message]),
        );
        draft.messagePrompts.bySession[sessionId] = sortPromptMessages(
          messages.map((message) =>
            existingByID.get(message.id)
              ? mergePromptMessage(existingByID.get(message.id)!, message)
              : message,
          ),
        );
        ensurePromptMeta(draft.messagePrompts.metaBySession, sessionId);
        if (meta) applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, meta);
      }),
    prependPromptMessages: (
      sessionId: string,
      messages: Parameters<SessionSlice["prependPromptMessages"]>[1],
      meta?: Parameters<SessionSlice["prependPromptMessages"]>[2],
    ) =>
      set((draft) => {
        const existing = draft.messagePrompts.bySession[sessionId] ?? [];
        const byID = new Map(existing.map((message) => [message.id, message]));
        for (const message of messages) {
          if (!isValidPrompt(message)) continue;
          const current = byID.get(message.id);
          byID.set(message.id, current ? mergePromptMessage(current, message) : message);
        }
        draft.messagePrompts.bySession[sessionId] = sortPromptMessages([...byID.values()]);
        ensurePromptMeta(draft.messagePrompts.metaBySession, sessionId);
        if (meta) applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, meta);
      }),
    setPromptMessagesLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, { isLoading: loading });
      }),
    setPromptMessagesLoadingMore: (sessionId: string, loading: boolean) =>
      set((draft) => {
        applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, {
          isLoadingMore: loading,
        });
      }),
  };
}
