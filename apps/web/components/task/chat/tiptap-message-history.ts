import { useCallback, useLayoutEffect, useMemo, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { createMessageHistorySelector, type MessageHistoryEntry } from "./message-history";

function useMessageHistoryForSession(sessionId: string | null): MessageHistoryEntry[] {
  const selector = useMemo(() => createMessageHistorySelector(sessionId), [sessionId]);
  return useAppStore(selector);
}

export function useChatHistory(sessionId: string | null) {
  const history = useMessageHistoryForSession(sessionId);
  const historyRef = useRef(history);
  useLayoutEffect(() => {
    historyRef.current = history;
  });
  const getHistory = useCallback(() => historyRef.current, []);
  return { history, getHistory };
}
