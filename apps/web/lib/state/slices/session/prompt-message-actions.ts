import type { StateCreator } from "zustand";
import type { Message } from "@/lib/types/http";
import type { SessionSlice, SessionSliceState } from "./types";
import {
  comparePromptOrder,
  isValidPromptMessage,
  type ObservedPrompts,
} from "@/lib/session-last-prompt";
import { isIncomingMessageAtLeastAsFresh } from "./message-timestamp";

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

function bumpPromptRevision(state: SessionSliceState, sessionId: string) {
  const revisions = (state.messagePrompts.refreshGenerationBySession ??= {});
  revisions[sessionId] = (revisions[sessionId] ?? 0) + 1;
}

/** Filters invalid rows and sorts prompts by creation order. */
function sortPromptMessages(messages: Message[]) {
  return messages
    .filter(isValidPromptMessage)
    .sort((left, right) => comparePromptOrder(left, right) ?? 0);
}

function acceptedPromptRows(state: SessionSliceState, sessionId: string, messages: Message[]) {
  const deleted = state.messagePrompts.deletedIdsBySession[sessionId];
  return messages.filter(
    (message) =>
      message.session_id === sessionId && isValidPromptMessage(message) && !deleted?.[message.id],
  );
}

function observePrompt(record: ObservedPrompts, message: Message) {
  record.ids[message.id] = true;
  if (!record.newestKey || comparePromptOrder(message, record.newestKey)! > 0) {
    record.newestKey = { id: message.id, created_at: message.created_at };
  }
}

/** Live creation is observed separately from snapshot and pagination fan-out. */
export function observeLivePrompt(state: SessionSliceState, message: Message) {
  if (
    !isValidPromptMessage(message) ||
    state.messagePrompts.deletedIdsBySession[message.session_id]?.[message.id]
  )
    return;
  const record = (state.messagePrompts.observedBySession[message.session_id] ??= {
    ids: {},
    newestKey: null,
  });
  observePrompt(record, message);
}

function repairPromptCursor(state: SessionSliceState, sessionId: string, pageSize: number) {
  const meta = state.messagePrompts.metaBySession[sessionId];
  const rows = state.messagePrompts.bySession[sessionId];
  if (pageSize === 0) return;
  if (rows.length === 0) {
    meta.oldestCursor = null;
    meta.hasMore = false;
  } else if (meta.oldestCursor && !rows.some((row) => row.id === meta.oldestCursor)) {
    meta.oldestCursor = rows[0].id;
  }
}

/** Merges a prompt update without regressing its immutable creation order. */
function mergePromptMessage(existing: Message, incoming: Message) {
  if (!isIncomingMessageAtLeastAsFresh(existing, incoming)) {
    return incoming.prompt_index !== undefined && existing.prompt_index === undefined
      ? { ...existing, prompt_index: incoming.prompt_index }
      : existing;
  }
  return {
    ...existing,
    ...incoming,
    created_at: existing.created_at,
    prompt_index: incoming.prompt_index ?? existing.prompt_index,
  };
}

/** Inserts or refreshes one prompt in the independent prompt cache. */
function upsertPromptMessage(state: SessionSliceState, message: Message) {
  if (!isValidPromptMessage(message)) return;
  if (state.messagePrompts.deletedIdsBySession[message.session_id]?.[message.id]) return;
  const sessionId = message.session_id;
  const prompts = state.messagePrompts.bySession[sessionId] ?? [];
  bumpPromptRevision(state, sessionId);
  const index = prompts.findIndex((prompt) => prompt.id === message.id);
  if (index === -1) prompts.push(message);
  else prompts[index] = mergePromptMessage(prompts[index], message);
  state.messagePrompts.bySession[sessionId] = sortPromptMessages(prompts);
  ensurePromptMeta(state.messagePrompts.metaBySession, sessionId);
}

/** Applies a live user-message update to the prompt cache when present. */
export function updatePromptMessage(state: SessionSliceState, message: Message) {
  if (!isValidPromptMessage(message)) return;
  if (state.messagePrompts.deletedIdsBySession[message.session_id]?.[message.id]) return;
  bumpPromptRevision(state, message.session_id);
  const prompts = state.messagePrompts.bySession[message.session_id] ?? [];
  const index = prompts.findIndex((entry) => entry.id === message.id);
  if (index === -1) prompts.push(message);
  else prompts[index] = mergePromptMessage(prompts[index], message);
  state.messagePrompts.bySession[message.session_id] = sortPromptMessages(prompts);
  ensurePromptMeta(state.messagePrompts.metaBySession, message.session_id);
}

/** Fans transcript message events into the prompt cache. */
export function fanOutTranscriptPrompts(state: SessionSliceState, messages: Message[]) {
  for (const message of messages) upsertPromptMessage(state, message);
}

/** Removes a prompt and repairs its cursor without discarding unloaded history. */
export function removePromptMessage(
  state: SessionSliceState,
  sessionId: string,
  messageId: string,
) {
  (state.messagePrompts.deletedIdsBySession[sessionId] ??= {})[messageId] = true;
  const observed = state.messagePrompts.observedBySession[sessionId];
  if (observed) delete observed.ids[messageId];
  bumpPromptRevision(state, sessionId);
  const prompts = state.messagePrompts.bySession[sessionId];
  if (!prompts) return;
  const nextPrompts = prompts.filter((message) => message.id !== messageId);
  if (nextPrompts.length === prompts.length) return;
  state.messagePrompts.bySession[sessionId] = nextPrompts;
  const meta = state.messagePrompts.metaBySession[sessionId];
  if (meta?.oldestCursor === messageId) {
    meta.oldestCursor = nextPrompts[0]?.id ?? null;
  }
  if (nextPrompts.length === 0 && meta && !meta.hasMore) {
    meta.oldestCursor = null;
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
        const accepted = acceptedPromptRows(draft, sessionId, messages);
        draft.messagePrompts.bySession[sessionId] = sortPromptMessages(
          accepted.map((message) => {
            const current = existingByID.get(message.id);
            return current ? mergePromptMessage(current, message) : message;
          }),
        );
        ensurePromptMeta(draft.messagePrompts.metaBySession, sessionId);
        if (meta) {
          applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, {
            ...meta,
            historyInitialized: true,
          });
        }
        repairPromptCursor(draft, sessionId, messages.length);
      }),
    prependPromptMessages: (
      sessionId: string,
      messages: Parameters<SessionSlice["prependPromptMessages"]>[1],
      meta?: Parameters<SessionSlice["prependPromptMessages"]>[2],
    ) =>
      set((draft) => {
        const existing = draft.messagePrompts.bySession[sessionId] ?? [];
        const byID = new Map(existing.map((message) => [message.id, message]));
        const accepted = acceptedPromptRows(draft, sessionId, messages);
        for (const message of accepted) {
          const current = byID.get(message.id);
          byID.set(message.id, current ? mergePromptMessage(current, message) : message);
        }
        draft.messagePrompts.bySession[sessionId] = sortPromptMessages([...byID.values()]);
        ensurePromptMeta(draft.messagePrompts.metaBySession, sessionId);
        if (meta) applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, meta);
        repairPromptCursor(draft, sessionId, messages.length);
      }),
    installAuthoritativePromptMessages: (
      sessionId: string,
      messages: Message[],
      meta: { hasMore: boolean; oldestCursor: string | null },
    ) =>
      set((draft) => {
        const byID = new Map(
          (draft.messagePrompts.bySession[sessionId] ?? []).map((message) => [message.id, message]),
        );
        const accepted = acceptedPromptRows(draft, sessionId, messages);
        for (const message of accepted) {
          const current = byID.get(message.id);
          byID.set(message.id, current ? mergePromptMessage(current, message) : message);
        }
        draft.messagePrompts.bySession[sessionId] = sortPromptMessages([...byID.values()]);
        const cursorWasDeleted =
          meta.oldestCursor !== null &&
          draft.messagePrompts.deletedIdsBySession[sessionId]?.[meta.oldestCursor];
        if (!draft.messagePrompts.metaBySession[sessionId]?.historyInitialized) {
          applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, {
            ...meta,
            oldestCursor: cursorWasDeleted ? null : meta.oldestCursor,
            historyInitialized: true,
          });
        }
        draft.messagePrompts.authoritativeBySession[sessionId] = true;
        const observed = (draft.messagePrompts.observedBySession[sessionId] ??= {
          ids: {},
          newestKey: null,
        });
        for (const message of accepted) observePrompt(observed, byID.get(message.id)!);
      }),
    setPromptMessagesLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        const promptMeta = draft.messagePrompts.metaBySession[sessionId];
        const wasLoading = promptMeta?.isLoading ?? false;
        applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, { isLoading: loading });
        if (loading && !wasLoading) {
          const generations = (draft.messagePrompts.refreshGenerationBySession ??= {});
          generations[sessionId] = (generations[sessionId] ?? 0) + 1;
        }
      }),
    setPromptMessagesLoadingMore: (sessionId: string, loading: boolean) =>
      set((draft) => {
        applyPromptMeta(draft.messagePrompts.metaBySession, sessionId, {
          isLoadingMore: loading,
        });
      }),
  };
}
