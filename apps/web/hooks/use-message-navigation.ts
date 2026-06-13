import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";

const EMPTY_KEY = "";

/**
 * Prev/next navigation between user messages in a session, sourced directly from
 * the store rather than a drilled `allMessages` array.
 *
 * The selector returns a value-stable key (the comma-joined ids of the session's
 * user messages). During agent token streaming the user-message set is unchanged,
 * so the key is referentially equal and the subscribing message does not
 * re-render — which keeps `ChatMessage` memoized instead of re-running its
 * markdown render on every streamed token.
 */
export function useUserMessageNavigation(sessionId: string | null, currentMessageId: string) {
  const userIdsKey = useAppStore((state) => {
    const messages = sessionId ? state.messages.bySession[sessionId] : undefined;
    if (!messages) return EMPTY_KEY;
    let key = EMPTY_KEY;
    for (const message of messages) {
      if (message.author_type === "user") key += `${message.id},`;
    }
    return key;
  });

  return useMemo(() => {
    const ids = userIdsKey ? userIdsKey.slice(0, -1).split(",") : [];
    const index = ids.indexOf(currentMessageId);
    const previousId = index > 0 ? ids[index - 1] : null;
    const nextId = index >= 0 && index < ids.length - 1 ? ids[index + 1] : null;
    return {
      hasPrevious: previousId !== null,
      hasNext: nextId !== null,
      previousId,
      nextId,
    };
  }, [userIdsKey, currentMessageId]);
}
