import { useMemo } from "react";
import type { Message } from "@/lib/types/http";

export function useMessageNavigation(
  messages: Message[],
  currentMessageId: string,
  filterType: "user" | "agent",
) {
  const currentIndex = useMemo(() => {
    return messages.findIndex((msg) => msg.id === currentMessageId);
  }, [messages, currentMessageId]);

  const filteredIndices = useMemo(() => {
    return messages
      .map((msg, idx) => (msg.author_type === filterType ? idx : -1))
      .filter((idx) => idx !== -1);
  }, [messages, filterType]);

  const previous = useMemo(() => {
    const validPrev = filteredIndices.filter((idx) => idx < currentIndex);
    return validPrev.length > 0 ? messages[validPrev[validPrev.length - 1]] : null;
  }, [filteredIndices, currentIndex, messages]);

  const next = useMemo(() => {
    const validNext = filteredIndices.filter((idx) => idx > currentIndex);
    return validNext.length > 0 ? messages[validNext[0]] : null;
  }, [filteredIndices, currentIndex, messages]);

  return {
    hasPrevious: previous !== null,
    hasNext: next !== null,
    previous,
    next,
  };
}
