import { useEffect, useMemo, useRef, useState } from "react";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";
import { getLastUserMessageId } from "@/components/task/chat/message-list-shared";
import type { Message } from "@/lib/types/http";

/**
 * Resolves the session's last user-authored message ("last prompt") even when
 * it is older than the loaded message window (e.g. an autonomous session whose
 * only user prompt is its task description, buried under hundreds of agent
 * messages).
 *
 * Prefers the window-derived message — always freshest, because a new user
 * message arrives via the store and is immediately present in `allMessages`.
 * Only when the window contains no user message at all does it issue a single
 * targeted fetch (`author_type=user`, newest first) instead of paginating
 * through the whole transcript. Returns the fetched message as a fallback so
 * the scroll-to-last-prompt affordances can render before the prompt row is
 * ever mounted.
 */
export function useLastUserMessage(
  sessionId: string | null,
  allMessages: Message[],
): { lastPromptMessage: Message | null } {
  const windowMessage = useMemo(() => {
    const id = getLastUserMessageId(allMessages);
    if (!id) return null;
    return allMessages.find((message) => message.id === id) ?? null;
  }, [allMessages]);

  const [fetchedMessage, setFetchedMessage] = useState<Message | null>(null);
  const fetchedSessionRef = useRef<string | null>(null);

  useEffect(() => {
    // A user message in the window is the authoritative answer and wins over
    // any earlier targeted fetch (it may be a brand-new prompt).
    if (windowMessage) {
      setFetchedMessage(null);
      fetchedSessionRef.current = null;
      return;
    }
    if (!sessionId || fetchedSessionRef.current === sessionId) return;
    fetchedSessionRef.current = sessionId;
    let cancelled = false;
    void listTaskSessionMessages(sessionId, {
      limit: 1,
      author_type: "user",
      sort: "desc",
    })
      .then((response) => {
        if (!cancelled) setFetchedMessage(response.messages[0] ?? null);
      })
      .catch(() => {
        if (!cancelled) setFetchedMessage(null);
      });
    return () => {
      cancelled = true;
    };
  }, [sessionId, windowMessage]);

  return { lastPromptMessage: windowMessage ?? fetchedMessage };
}
