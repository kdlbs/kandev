/* eslint-disable max-lines -- The facade hook centralizes ordered snapshot/live state coordination. */
"use client";

import * as React from "react";
import { useMessageFavoritesStore } from "@/lib/state/slices/message-favorites";
import {
  compareConversationMessages,
  eventMatchesTask,
  eventString,
  messageFromEvent,
  turnFromEvent,
} from "./conversation-event-projection";
import type {
  PluginConversationApi,
  PluginConversationError,
  PluginConversationMessage,
  PluginConversationTurn,
  PluginSessionMessagesQuery,
  PluginSessionMessagesState,
  PluginSessionTurnsState,
} from "./types";
import {
  ConversationScopeContext,
  parseConversationResponse,
  pluginConversationUrl,
  type ConversationScope,
} from "./conversation-scope";
export { PluginConversationScopeProvider } from "./conversation-scope";

type MessagePage = {
  messages: PluginConversationMessage[];
  hasMore: boolean;
  cursor: string | null;
};

type TurnsPage = { turns: PluginConversationTurn[] };

const EMPTY_MESSAGES: PluginSessionMessagesState = {
  messages: [],
  loading: false,
  hydrated: false,
  loadingMore: false,
  error: null,
  hasMore: false,
  removed: false,
  loadMore: async () => 0,
  retry: () => undefined,
};

const EMPTY_TURNS: PluginSessionTurnsState = {
  turns: [],
  loading: false,
  hydrated: false,
  error: null,
  removed: false,
  retry: () => undefined,
};

function conversationError(error: unknown): PluginConversationError {
  if (typeof error === "object" && error !== null && "code" in error) {
    return error as PluginConversationError;
  }
  return {
    code: "upstream_failure",
    // i18n-exempt: plugin-facing transport diagnostic; plugins render their own localized error UI.
    message: error instanceof Error ? error.message : "Conversation request failed",
    retryable: true,
  };
}

function isRemovedConversationError(error: PluginConversationError): boolean {
  return error.code === "not_found";
}

function resolveTaskId(
  scope: ConversationScope,
  taskId: string | null | undefined,
): { taskId: string | null; error: PluginConversationError | null } {
  const resolved = taskId === undefined ? scope.taskId : taskId;
  if (resolved === "" || (typeof resolved === "string" && resolved !== scope.taskId)) {
    return {
      taskId: null,
      error: {
        code: "invalid_query",
        // i18n-exempt: plugin-facing programmer diagnostic; plugins render their own localized error UI.
        message: "taskId must match the active task",
        retryable: false,
      },
    };
  }
  return { taskId: resolved, error: null };
}

type MessagesState = Omit<PluginSessionMessagesState, "loadMore" | "retry">;
type MessagesSetter = React.Dispatch<React.SetStateAction<MessagesState>>;
function useOrderedMessageEvents({
  scope,
  sessionId,
  taskId,
  authorTypes,
  authorsKey,
  sort,
  snapshotKey,
  setState,
  cursorRef,
  messagesRef,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  taskId: string | null;
  authorTypes?: readonly ("user" | "agent")[];
  authorsKey: string;
  sort: "asc" | "desc";
  snapshotKey: string;
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  messagesRef: React.MutableRefObject<readonly PluginConversationMessage[]>;
}) {
  React.useEffect(() => {
    if (!scope || !sessionId) return;
    return scope.subscribe(
      (event) => {
        if (event.event_type === "session.removed") {
          setState((current) => ({ ...current, removed: true, hasMore: false, loading: false }));
          cursorRef.current = null;
          return true;
        }
        if (!event.event_type.startsWith("message.") || !eventMatchesTask(event, taskId)) {
          return true;
        }
        const messageId = eventString(event, "message_id");
        if (!messageId) return false;
        if (event.event_type === "message.deleted") {
          setState((current) => {
            const messages = current.messages.filter((message) => message.id !== messageId);
            messagesRef.current = messages;
            return { ...current, messages };
          });
          return true;
        }
        const message = messageFromEvent(event);
        if (!message) return false;
        if (authorTypes && !authorTypes.includes(message.authorType)) return true;
        setState((current) => {
          const messages = current.messages.filter((item) => item.id !== message.id);
          messages.push(message);
          messages.sort((left, right) => compareConversationMessages(left, right, sort));
          messagesRef.current = messages;
          return { ...current, messages };
        });
        return true;
      },
      "messages",
      snapshotKey,
    );
  }, [
    authorTypes,
    authorsKey,
    cursorRef,
    messagesRef,
    scope,
    sessionId,
    setState,
    snapshotKey,
    sort,
    taskId,
  ]);
}

function useInitialMessagePage({
  scope,
  sessionId,
  error,
  revision,
  loadPage,
  setState,
  cursorRef,
  loadMoreRef,
  messagesRef,
  requestRevisionRef,
  snapshotKey,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  error: PluginConversationError | null;
  revision: number;
  loadPage: (cursor: string | null, append: boolean) => Promise<number>;
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  loadMoreRef: React.MutableRefObject<Promise<number> | null>;
  messagesRef: React.MutableRefObject<readonly PluginConversationMessage[]>;
  requestRevisionRef: React.MutableRefObject<number>;
  snapshotKey: string;
}) {
  React.useEffect(() => {
    requestRevisionRef.current += 1;
    cursorRef.current = null;
    loadMoreRef.current = null;
    messagesRef.current = [];
    if (!scope || !sessionId) {
      setState({ ...EMPTY_MESSAGES, messages: [] });
      return;
    }
    if (error) {
      setState({ ...EMPTY_MESSAGES, messages: [], error });
      return;
    }
    void loadPage(null, false).catch((cause: unknown) => {
      if (scope.signal.aborted || scope.isTerminal()) return;
      const error = conversationError(cause);
      if (isRemovedConversationError(error)) {
        setState((current) => ({
          ...current,
          loading: false,
          loadingMore: false,
          error: null,
          hasMore: false,
          removed: true,
        }));
        return;
      }
      setState({ ...EMPTY_MESSAGES, messages: [], error });
    });
  }, [
    cursorRef,
    error,
    loadMoreRef,
    loadPage,
    messagesRef,
    requestRevisionRef,
    revision,
    scope,
    sessionId,
    setState,
    snapshotKey,
  ]);
}
function invalidateFreshMessagesSnapshot(
  scope: ConversationScope,
  append: boolean,
  snapshotKey: string,
): void {
  if (append) return;
  scope.invalidateSnapshot("messages", snapshotKey);
}

function isScopeRequestable(scope: ConversationScope): boolean {
  return !scope.signal.aborted && !scope.isTerminal();
}

function canLoadMore(
  scope: ConversationScope | null,
  sessionId: string | null,
  state: { removed: boolean; hasMore: boolean },
  cursor: string | null,
): boolean {
  return Boolean(
    scope && sessionId && isScopeRequestable(scope) && !state.removed && state.hasMore && cursor,
  );
}

type MessagePageLoaderOptions = {
  scope: ConversationScope | null;
  sessionId: string | null;
  taskId: string | null;
  error: PluginConversationError | null;
  authorsKey: string;
  sort: "asc" | "desc";
  limit: number;
  snapshotKey: string;
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  messagesRef: React.MutableRefObject<readonly PluginConversationMessage[]>;
  requestRevisionRef: React.MutableRefObject<number>;
};

async function fetchMessagePage({
  scope,
  sessionId,
  taskId,
  authorsKey,
  sort,
  limit,
  cursor,
  binding,
}: {
  scope: ConversationScope;
  sessionId: string;
  taskId: string | null;
  authorsKey: string;
  sort: "asc" | "desc";
  limit: number;
  cursor: string | null;
  binding: { bindingToken: string; snapshotToken: string };
}): Promise<MessagePage> {
  const params = new URLSearchParams({ sort, limit: String(limit) });
  if (taskId !== null) params.set("task_id", taskId);
  for (const author of authorsKey ? authorsKey.split(",") : []) {
    params.append("author_type", author);
  }
  if (cursor) params.set("cursor", cursor);
  const response = await fetch(
    pluginConversationUrl(
      scope.pluginId,
      `/conversation/task-sessions/${encodeURIComponent(sessionId)}/messages?${params}`,
    ),
    {
      credentials: "include",
      cache: "no-store",
      headers: {
        "X-Kandev-Plugin-Binding": binding.bindingToken,
        "X-Kandev-Snapshot-Token": binding.snapshotToken,
      },
      signal: scope.signal,
    },
  );
  return parseConversationResponse<MessagePage>(response);
}

function mergeMessagePage(
  current: readonly PluginConversationMessage[],
  page: readonly PluginConversationMessage[],
  append: boolean,
  sort: "asc" | "desc",
): { messages: PluginConversationMessage[]; additionCount: number } {
  const existingIds = new Set(current.map((message) => message.id));
  const messagesById = new Map<string, PluginConversationMessage>();
  if (append) {
    for (const message of current) messagesById.set(message.id, message);
  }
  for (const message of page) {
    const previous = messagesById.get(message.id);
    if (!previous || previous.updatedAt.localeCompare(message.updatedAt) < 0) {
      messagesById.set(message.id, message);
    }
  }
  const messages = Array.from(messagesById.values());
  messages.sort((left, right) => compareConversationMessages(left, right, sort));
  return {
    messages,
    additionCount: page.filter((message) => !existingIds.has(message.id)).length,
  };
}

function useMessagePageLoader({
  scope,
  sessionId,
  taskId,
  error,
  authorsKey,
  sort,
  limit,
  snapshotKey,
  setState,
  cursorRef,
  messagesRef,
  requestRevisionRef,
}: MessagePageLoaderOptions) {
  return React.useCallback(
    async (cursor: string | null, append: boolean): Promise<number> => {
      if (!scope || !sessionId || !isScopeRequestable(scope)) return 0;
      if (error) throw error;
      // A fresh full page (initial load, retry, or binding refresh) must not
      // let a concurrent live update project and then be overwritten by the
      // stale page response: buffer live events until the new snapshot
      // commits, then drain them on top.
      invalidateFreshMessagesSnapshot(scope, append, snapshotKey);
      const capturedRevision = requestRevisionRef.current;
      let binding = await scope.ready();
      let pageCursor = cursor;
      if (pageCursor) {
        const renewal = await scope.renewContinuation(pageCursor, snapshotKey);
        pageCursor = renewal.cursor;
        binding = renewal.binding;
      }
      const page = await fetchMessagePage({
        scope,
        sessionId,
        taskId,
        authorsKey,
        sort,
        limit,
        cursor: pageCursor,
        binding,
      });
      if (requestRevisionRef.current !== capturedRevision || scope.signal.aborted) {
        return 0;
      }
      const { messages, additionCount } = mergeMessagePage(
        messagesRef.current,
        page.messages,
        append,
        sort,
      );
      messagesRef.current = messages;
      setState((current) => ({
        ...current,
        messages,
        loading: false,
        hydrated: true,
        loadingMore: false,
        error: null,
        hasMore: page.hasMore,
      }));
      if (!append) scope.commitSnapshot("messages", snapshotKey);
      cursorRef.current = page.cursor;
      return additionCount;
    },
    [
      authorsKey,
      error,
      limit,
      messagesRef,
      requestRevisionRef,
      scope,
      snapshotKey,
      sessionId,
      setState,
      sort,
      taskId,
    ],
  );
}

function useMessageRebind({
  scope,
  sessionId,
  error,
  loadPage,
  setState,
  cursorRef,
  loadMoreRef,
  requestRevisionRef,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  error: PluginConversationError | null;
  loadPage: (cursor: string | null, append: boolean) => Promise<number>;
  setState: MessagesSetter;
  cursorRef: React.MutableRefObject<string | null>;
  loadMoreRef: React.MutableRefObject<Promise<number> | null>;
  requestRevisionRef: React.MutableRefObject<number>;
}) {
  React.useEffect(() => {
    if (!scope || !sessionId || error) return;
    return scope.subscribeRebind(() => {
      requestRevisionRef.current += 1;
      cursorRef.current = null;
      loadMoreRef.current = null;
      setState((current) => ({ ...current, loading: true, loadingMore: false, error: null }));
      void loadPage(null, false).catch((cause: unknown) => {
        if (scope.signal.aborted || scope.isTerminal()) return;
        setState((current) => ({
          ...current,
          loading: false,
          error: conversationError(cause),
        }));
      });
    });
  }, [error, loadMoreRef, loadPage, requestRevisionRef, scope, sessionId, setState, cursorRef]);
}

function useMessageControls({
  scope,
  sessionId,
  state,
  setState,
  loadPage,
  cursorRef,
  loadMoreRef,
  setRevision,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  state: MessagesState;
  setState: MessagesSetter;
  loadPage: (cursor: string | null, append: boolean) => Promise<number>;
  cursorRef: React.MutableRefObject<string | null>;
  loadMoreRef: React.MutableRefObject<Promise<number> | null>;
  setRevision: React.Dispatch<React.SetStateAction<number>>;
}): PluginSessionMessagesState {
  const loadMore = React.useCallback((): Promise<number> => {
    if (!canLoadMore(scope, sessionId, state, cursorRef.current)) return Promise.resolve(0);
    if (loadMoreRef.current) return loadMoreRef.current;
    setState((current) => ({ ...current, loadingMore: true }));
    const request = loadPage(cursorRef.current, true)
      .catch((error: unknown) => {
        setState((current) => ({
          ...current,
          loadingMore: false,
          error: conversationError(error),
        }));
        throw conversationError(error);
      })
      .finally(() => {
        if (loadMoreRef.current === request) loadMoreRef.current = null;
      });
    loadMoreRef.current = request;
    return request;
  }, [cursorRef, loadMoreRef, loadPage, scope, sessionId, setState, state.hasMore, state.removed]);

  const retry = React.useCallback(() => {
    if (!state.error?.retryable || state.removed || !scope || scope.isTerminal() || !sessionId)
      return;
    setRevision((value) => value + 1);
  }, [scope, sessionId, setRevision, state.error, state.removed]);

  return React.useMemo(() => ({ ...state, loadMore, retry }), [loadMore, retry, state]);
}

function useSessionMessages(query: PluginSessionMessagesQuery): PluginSessionMessagesState {
  const scope = React.useContext(ConversationScopeContext);
  const [revision, setRevision] = React.useState(0);
  const [state, setState] = React.useState<Omit<PluginSessionMessagesState, "loadMore" | "retry">>({
    ...EMPTY_MESSAGES,
  });
  const cursorRef = React.useRef<string | null>(null);
  const loadMoreRef = React.useRef<Promise<number> | null>(null);
  const messagesRef = React.useRef<readonly PluginConversationMessage[]>([]);
  const requestRevisionRef = React.useRef(0);
  const resolved = React.useMemo(
    () => (scope ? resolveTaskId(scope, query.taskId) : { taskId: null, error: null }),
    [query.taskId, scope],
  );
  const authorsKey = [...(query.authorTypes ?? [])].join(",");
  const sort = query.sort ?? "desc";
  const limit = query.pageSize ?? 20;
  const snapshotKey = JSON.stringify([query.sessionId, resolved.taskId, authorsKey, sort, limit]);

  const loadPage = useMessagePageLoader({
    scope,
    sessionId: query.sessionId,
    taskId: resolved.taskId,
    error: resolved.error,
    authorsKey,
    sort,
    limit,
    snapshotKey,
    setState,
    cursorRef,
    messagesRef,
    requestRevisionRef,
  });
  useOrderedMessageEvents({
    scope: resolved.error ? null : scope,
    sessionId: query.sessionId,
    taskId: resolved.taskId,
    authorTypes: query.authorTypes,
    authorsKey,
    sort,
    snapshotKey,
    setState,
    cursorRef,
    messagesRef,
  });

  useInitialMessagePage({
    scope,
    sessionId: query.sessionId,
    error: resolved.error,
    revision,
    loadPage,
    setState,
    cursorRef,
    loadMoreRef,
    messagesRef,
    requestRevisionRef,
    snapshotKey,
  });

  useMessageRebind({
    scope,
    sessionId: query.sessionId,
    error: resolved.error,
    loadPage,
    setState,
    cursorRef,
    loadMoreRef,
    requestRevisionRef,
  });
  return useMessageControls({
    scope,
    sessionId: query.sessionId,
    state,
    setState,
    loadPage,
    cursorRef,
    loadMoreRef,
    setRevision,
  });
}

type TurnsState = Omit<PluginSessionTurnsState, "retry">;
type TurnsSetter = React.Dispatch<React.SetStateAction<TurnsState>>;

function useOrderedTurnEvents({
  scope,
  sessionId,
  taskId,
  error,
  snapshotKey,
  setState,
}: {
  scope: ConversationScope | null;
  sessionId: string | null;
  taskId: string | null;
  error: PluginConversationError | null;
  snapshotKey: string;
  setState: TurnsSetter;
}) {
  React.useEffect(() => {
    if (!scope || !sessionId || error) return;
    return scope.subscribe(
      (event) => {
        if (event.event_type === "session.removed") {
          setState((current) => ({ ...current, removed: true, loading: false }));
          return true;
        }
        if (!eventMatchesTask(event, taskId)) return true;
        if (event.event_type === "session.turn.removed") {
          const payload = event.payload;
          const turnId =
            payload &&
            typeof payload === "object" &&
            "id" in payload &&
            typeof payload.id === "string"
              ? payload.id
              : undefined;
          if (!turnId) return false;
          setState((current) => ({
            ...current,
            turns: current.turns.filter((item) => item.id !== turnId),
          }));
          return true;
        }
        if (!event.event_type.startsWith("session.turn.")) return true;
        const turn = turnFromEvent(event);
        if (!turn) return false;
        setState((current) => {
          const turns = current.turns.filter((item) => item.id !== turn.id);
          turns.push(turn);
          turns.sort((left, right) => {
            const started = left.startedAt.localeCompare(right.startedAt);
            return started === 0 ? left.id.localeCompare(right.id) : started;
          });
          return { ...current, turns, hydrated: true };
        });
        return true;
      },
      "turns",
      snapshotKey,
    );
  }, [error, scope, sessionId, setState, snapshotKey, taskId]);
}
// eslint-disable-next-line max-lines-per-function -- keeps turn snapshot lifecycle in one hook.
function useSessionTurns(
  sessionId: string | null,
  taskId?: string | null,
): PluginSessionTurnsState {
  const scope = React.useContext(ConversationScopeContext);
  const [revision, setRevision] = React.useState(0);
  const [state, setState] = React.useState<Omit<PluginSessionTurnsState, "retry">>({
    ...EMPTY_TURNS,
    turns: [],
  });
  const resolved = React.useMemo(
    () => (scope ? resolveTaskId(scope, taskId) : { taskId: null, error: null }),
    [scope, taskId],
  );
  const snapshotKey = JSON.stringify([sessionId, resolved.taskId]);

  useOrderedTurnEvents({
    scope,
    sessionId,
    taskId: resolved.taskId,
    error: resolved.error,
    snapshotKey,
    setState,
  });

  React.useEffect(() => {
    if (!scope || !sessionId) {
      setState({ ...EMPTY_TURNS, turns: [] });
      return;
    }
    if (resolved.error) {
      setState({ ...EMPTY_TURNS, turns: [], error: resolved.error });
      return;
    }
    let current = true;
    // A fresh turns page (initial load, retry, or binding refresh) must not
    // let a concurrent live turn event project and then be overwritten by the
    // stale page response: buffer live events until the new snapshot commits,
    // then drain them on top.
    scope.invalidateSnapshot("turns", snapshotKey);
    setState((previous) =>
      revision === 0
        ? { ...EMPTY_TURNS, turns: [], loading: true }
        : { ...previous, loading: true, error: null },
    );
    void scope
      .ready()
      .then(async (binding) => {
        const params = new URLSearchParams();
        if (resolved.taskId !== null) params.set("task_id", resolved.taskId);
        const queryString = params.size ? `?${params}` : "";
        const response = await fetch(
          pluginConversationUrl(
            scope.pluginId,
            `/conversation/task-sessions/${encodeURIComponent(sessionId)}/turns${queryString}`,
          ),
          {
            credentials: "include",
            cache: "no-store",
            headers: {
              "X-Kandev-Plugin-Binding": binding.bindingToken,
              "X-Kandev-Snapshot-Token": binding.snapshotToken,
            },
            signal: scope.signal,
          },
        );
        return parseConversationResponse<TurnsPage>(response);
      })
      .then((page) => {
        if (!current || scope.signal.aborted) return;
        setState((previous) => ({
          ...previous,
          turns: page.turns,
          loading: false,
          hydrated: true,
          error: null,
        }));
        scope.commitSnapshot("turns", snapshotKey);
      })
      .catch((cause: unknown) => {
        if (!current || scope.signal.aborted || scope.isTerminal()) return;
        const error = conversationError(cause);
        if (isRemovedConversationError(error)) {
          setState((previous) => ({
            ...previous,
            loading: false,
            error: null,
            removed: true,
          }));
          return;
        }
        setState((previous) => ({
          ...(revision === 0 ? { ...EMPTY_TURNS, turns: [] } : previous),
          loading: false,
          error,
        }));
      });
    return () => {
      current = false;
    };
  }, [resolved.error, resolved.taskId, revision, scope, sessionId, snapshotKey]);

  React.useEffect(() => {
    if (!scope || !sessionId || resolved.error) return;
    return scope.subscribeRebind(() => setRevision((value) => value + 1));
  }, [resolved.error, scope, sessionId]);

  const retry = React.useCallback(() => {
    if (!state.error?.retryable || state.removed || !scope || !sessionId) return;
    setRevision((value) => value + 1);
  }, [scope, sessionId, state.error, state.removed]);

  return React.useMemo(() => ({ ...state, retry }), [retry, state]);
}

function useMessageFavorite(sessionId: string | null, messageId: string): boolean {
  const hydrateSession = useMessageFavoritesStore((state) => state.hydrateSession);
  React.useEffect(() => {
    if (sessionId) hydrateSession(sessionId);
  }, [hydrateSession, sessionId]);
  return useMessageFavoritesStore((state) =>
    sessionId ? Boolean(state.bySession[sessionId]?.[messageId]) : false,
  );
}

export const pluginConversationApi: PluginConversationApi = {
  useSessionMessages,
  useSessionTurns,
  useMessageFavorite,
};
