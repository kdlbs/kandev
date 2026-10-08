"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import {
  executeUtilityPrompt,
  type ExecutePromptRequest,
  type ExecutePromptResponse,
} from "@/lib/api/domains/utility-api";
import {
  PROMPT_SUGGESTION_TIMEOUT_MS,
  SUGGEST_NEXT_PROMPT_AGENT_ID,
  buildSuggestionTranscript,
  isNativeSuggestionSession,
  lastEntryIsAgent,
  normalizeFallbackSuggestion,
  readNativePromptSuggestion,
  readSuggestionCache,
  resolveSuggestionTurnKey,
  shouldRequestFallback,
  writeSuggestionCache,
  type FallbackEligibility,
  type SuggestionCacheEntry,
} from "@/lib/prompt-suggestion";
import type { Message, TaskSession, Turn } from "@/lib/types/http";

export type PromptSuggestionInputs = {
  sessionId: string | null;
  taskTitle: string;
  enabled: boolean;
  fallbackEnabled: boolean;
  native: boolean;
  session: TaskSession | null;
  turns: readonly Turn[] | undefined;
  activeTurnId: string | null | undefined;
  messages: readonly Message[] | undefined;
  queued: boolean;
  /** True while the composer is busy, recovering, or waiting on a clarification. */
  blocked: boolean;
};

export type PromptSuggestionRequester = (
  request: ExecutePromptRequest,
  signal: AbortSignal,
) => Promise<ExecutePromptResponse>;

const defaultRequester: PromptSuggestionRequester = (request, signal) =>
  executeUtilityPrompt(request, { init: { signal } });

function isWaitingForInput(input: PromptSuggestionInputs): boolean {
  const session = input.session;
  return (
    session?.state === "WAITING_FOR_INPUT" &&
    !session.error_message &&
    !input.activeTurnId &&
    !input.blocked &&
    !input.queued
  );
}

function visibleNativeSuggestion(
  input: PromptSuggestionInputs,
  turnKey: string | null,
  entry: SuggestionCacheEntry | null,
): string | null {
  const native = readNativePromptSuggestion(input.session?.metadata);
  if (!native) return null;
  // Without loaded turns the backend's state-guarded turn binding is authoritative.
  const turnMatches = !input.turns?.length || native.turnKey === turnKey;
  const dismissed = Boolean(entry?.dismissed) && entry?.turnKey === native.turnKey;
  return turnMatches && !dismissed ? native.text : null;
}

function visibleSuggestion(
  input: PromptSuggestionInputs,
  turnKey: string | null,
  entry: SuggestionCacheEntry | null,
): string | null {
  if (!input.enabled || !isWaitingForInput(input)) return null;
  if (input.native) return visibleNativeSuggestion(input, turnKey, entry);
  if (!turnKey || !input.fallbackEnabled || entry?.dismissed) return null;
  return entry?.text ?? null;
}

function fallbackEligibility(
  input: PromptSuggestionInputs,
  turnKey: string | null,
  cached: boolean,
  inFlight: boolean,
): FallbackEligibility {
  return {
    enabled: input.enabled,
    fallbackEnabled: input.fallbackEnabled,
    native: input.native,
    sessionState: input.session?.state,
    needsRecovery: Boolean(input.session?.error_message),
    activeTurn: Boolean(input.activeTurnId),
    pendingClarification: input.blocked,
    queued: input.queued,
    lastEntryIsAgent: lastEntryIsAgent(input.messages),
    turnKey,
    cached,
    inFlight,
  };
}

function nativeSuggestionTurnKey(input: PromptSuggestionInputs): string | undefined {
  return input.native ? readNativePromptSuggestion(input.session?.metadata)?.turnKey : undefined;
}

function useSuggestionCache(sessionId: string | null) {
  const [entry, setEntry] = useState<SuggestionCacheEntry | null>(() =>
    sessionId ? readSuggestionCache(sessionId) : null,
  );
  const sessionRef = useRef(sessionId);
  useEffect(() => {
    sessionRef.current = sessionId;
    setEntry(sessionId ? readSuggestionCache(sessionId) : null);
  }, [sessionId]);
  const save = useCallback((next: SuggestionCacheEntry) => {
    writeSuggestionCache(next);
    if (sessionRef.current === next.sessionId) setEntry(next);
  }, []);
  return { entry, save };
}

function startFallbackRequest(
  input: PromptSuggestionInputs,
  request: PromptSuggestionRequester,
  finish: (text: string | null) => void,
  release: () => void,
) {
  const controller = new AbortController();
  let settled = false;
  const settle = (text: string | null, cache = true) => {
    if (settled) return;
    settled = true;
    clearTimeout(timer);
    if (cache) finish(text);
    else release();
  };
  const timer = setTimeout(() => {
    controller.abort();
    settle(null);
  }, PROMPT_SUGGESTION_TIMEOUT_MS);
  request(
    {
      utility_agent_id: SUGGEST_NEXT_PROMPT_AGENT_ID,
      session_id: "",
      task_title: input.taskTitle,
      conversation_history: buildSuggestionTranscript(input.messages ?? []),
      // Used only when no suggestion or default utility profile is configured.
      ...(input.session?.agent_profile_id
        ? { fallback_agent_profile_id: input.session.agent_profile_id }
        : {}),
    },
    controller.signal,
  ).then(
    (response) => settle(response.success ? normalizeFallbackSuggestion(response.response) : null),
    // Only a server answer is final for the turn; a request that never reached
    // the server (page unload, network loss) may be retried on the next mount.
    (error: unknown) => settle(null, error instanceof ApiError),
  );
}

/** Selects the visible next-prompt suggestion and runs the utility fallback when eligible. */
export function usePromptSuggestionController(
  input: PromptSuggestionInputs,
  request: PromptSuggestionRequester = defaultRequester,
) {
  const { sessionId, turns, messages } = input;
  const turnKey = useMemo(() => resolveSuggestionTurnKey(turns, messages), [turns, messages]);
  const { entry, save } = useSuggestionCache(sessionId);
  const entryForTurn =
    entry && entry.sessionId === sessionId && entry.turnKey === turnKey ? entry : null;
  const inFlight = useRef<string | null>(null);
  const key = sessionId && turnKey ? `${sessionId}:${turnKey}` : null;

  const shouldRequest = shouldRequestFallback(
    fallbackEligibility(
      input,
      turnKey,
      Boolean(entryForTurn),
      key !== null && inFlight.current === key,
    ),
  );

  const latestInput = useRef(input);
  latestInput.current = input;
  useEffect(() => {
    if (!shouldRequest || !sessionId || !turnKey || !key || inFlight.current === key) return;
    inFlight.current = key;
    const releaseKey = () => {
      if (inFlight.current === key) inFlight.current = null;
    };
    startFallbackRequest(
      latestInput.current,
      request,
      (text) => {
        releaseKey();
        save({ sessionId, turnKey, text, dismissed: false });
      },
      releaseKey,
    );
  }, [shouldRequest, sessionId, turnKey, key, request, save]);

  const nativeTurnKey = nativeSuggestionTurnKey(input);
  const dismissKey = nativeTurnKey ?? turnKey;
  const dismiss = useCallback(() => {
    if (!sessionId || !dismissKey) return;
    save({ sessionId, turnKey: dismissKey, text: entryForTurn?.text ?? null, dismissed: true });
  }, [sessionId, dismissKey, entryForTurn, save]);

  const visibleEntry = nativeTurnKey && entry?.turnKey === nativeTurnKey ? entry : entryForTurn;
  return { suggestion: visibleSuggestion(input, turnKey, visibleEntry), dismiss };
}

type UsePromptSuggestionOptions = {
  sessionId: string | null;
  taskTitle: string;
  blocked: boolean;
};

/** Store-backed prompt suggestion for the shared chat composer. */
export function usePromptSuggestion({ sessionId, taskTitle, blocked }: UsePromptSuggestionOptions) {
  const enabled = useAppStore((state) => state.userSettings.promptSuggestions);
  const fallbackEnabled = useAppStore((state) => state.userSettings.promptSuggestionsFallback);
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const turns = useAppStore((state) => (sessionId ? state.turns.bySession[sessionId] : undefined));
  const activeTurnId = useAppStore((state) =>
    sessionId ? state.turns.activeBySession[sessionId] : null,
  );
  const messages = useAppStore((state) =>
    sessionId ? state.messages.bySession[sessionId] : undefined,
  );
  const queued = useAppStore((state) =>
    sessionId ? (state.queue.bySessionId[sessionId]?.length ?? 0) > 0 : false,
  );
  const native = isNativeSuggestionSession(session);
  return usePromptSuggestionController({
    sessionId,
    taskTitle,
    enabled,
    fallbackEnabled,
    native,
    session,
    turns,
    activeTurnId,
    messages,
    queued,
    blocked,
  });
}
