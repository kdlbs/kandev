"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  readSuggestionDismissal,
  resolveSuggestionTurnKey,
  selectVisibleSuggestion,
  writeSuggestionDismissal,
} from "@/lib/prompt-suggestion";
import type { Message, TaskSession, Turn } from "@/lib/types/http";

export type PromptSuggestionInputs = {
  sessionId: string | null;
  enabled: boolean;
  session: TaskSession | null;
  turns: readonly Turn[] | undefined;
  activeTurnId: string | null | undefined;
  messages: readonly Message[] | undefined;
  queued: boolean;
  /** True while the composer is busy, recovering, moving, or waiting on a clarification. */
  blocked: boolean;
};

function useDismissedTurnKey(sessionId: string | null) {
  const read = useCallback(
    () => (sessionId ? (readSuggestionDismissal(sessionId)?.turnKey ?? null) : null),
    [sessionId],
  );
  const [dismissedTurnKey, setDismissedTurnKey] = useState<string | null>(read);
  useEffect(() => setDismissedTurnKey(read()), [read]);
  const dismissTurn = useCallback(
    (turnKey: string) => {
      if (!sessionId) return;
      writeSuggestionDismissal({ sessionId, turnKey });
      setDismissedTurnKey(turnKey);
    },
    [sessionId],
  );
  return { dismissedTurnKey, dismissTurn };
}

/** Selects the visible native next-prompt suggestion. It never requests one. */
export function usePromptSuggestionController(input: PromptSuggestionInputs) {
  const { sessionId, turns, messages } = input;
  const turnKey = useMemo(() => resolveSuggestionTurnKey(turns, messages), [turns, messages]);
  const { dismissedTurnKey, dismissTurn } = useDismissedTurnKey(sessionId);
  const visible = selectVisibleSuggestion({
    enabled: input.enabled,
    session: input.session,
    turnKey,
    activeTurn: Boolean(input.activeTurnId),
    blocked: input.blocked,
    queued: input.queued,
    dismissedTurnKey,
  });
  const visibleTurnKey = visible?.turnKey ?? null;
  const dismiss = useCallback(() => {
    if (visibleTurnKey) dismissTurn(visibleTurnKey);
  }, [visibleTurnKey, dismissTurn]);
  return { suggestion: visible?.text ?? null, dismiss };
}

type UsePromptSuggestionOptions = {
  sessionId: string | null;
  blocked: boolean;
};

/** Store-backed prompt suggestion for the shared chat composer. */
export function usePromptSuggestion({ sessionId, blocked }: UsePromptSuggestionOptions) {
  const enabled = useAppStore((state) => state.userSettings.promptSuggestions);
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
  return usePromptSuggestionController({
    sessionId,
    enabled,
    session,
    turns,
    activeTurnId,
    messages,
    queued,
    blocked,
  });
}
