import { useMemo, useRef, useState } from "react";
import type { useAppStoreApi } from "@/components/state-provider";

export function useMessageFetchState(store: ReturnType<typeof useAppStoreApi>) {
  const [isLoading, setIsLoading] = useState(false);
  const [isWaitingForInitialMessages, setIsWaitingForInitialMessages] = useState(false);
  const [isCachedHistoryRefreshPending, setIsCachedHistoryRefreshPending] = useState(false);
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
    }),
    [store],
  );
  return {
    isLoading,
    isWaitingForInitialMessages,
    isCachedHistoryRefreshPending,
    setIsCachedHistoryRefreshPending,
    setIsWaitingForInitialMessages,
    initialFetchStartRef,
    lastFetchedSessionIdRef,
    cachedRefreshGenerationRef,
    refs,
  };
}
