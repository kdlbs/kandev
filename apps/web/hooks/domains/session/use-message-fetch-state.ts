import { useMemo, useRef, useState } from "react";
import type { useAppStoreApi } from "@/components/state-provider";

export type MessageHistoryStatus = "loading" | "retrying" | "ready" | "unavailable";

export function useMessageFetchState(store: ReturnType<typeof useAppStoreApi>) {
  const [isLoading, setIsLoading] = useState(false);
  const [isWaitingForInitialMessages, setIsWaitingForInitialMessages] = useState(false);
  const [isCachedHistoryRefreshPending, setIsCachedHistoryRefreshPending] = useState(false);
  const [historyStatus, setHistoryStatus] = useState<MessageHistoryStatus>("loading");
  const [historyError, setHistoryError] = useState<unknown>(null);
  const cachedRefreshGenerationRef = useRef(0);
  const initialFetchStartRef = useRef<number | null>(null);
  const lastFetchedSessionIdRef = useRef<string | null>(null);
  const refs = useMemo(
    () => ({
      store,
      setIsLoading,
      setIsWaitingForInitialMessages,
      initialFetchStartRef,
      lastFetchedSessionIdRef,
      setHistoryStatus,
      setHistoryError,
    }),
    [store],
  );
  return {
    isLoading,
    isWaitingForInitialMessages,
    isCachedHistoryRefreshPending,
    setIsCachedHistoryRefreshPending,
    historyStatus,
    setHistoryStatus,
    historyError,
    setHistoryError,
    setIsWaitingForInitialMessages,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    cachedRefreshGenerationRef,
    refs,
  };
}
