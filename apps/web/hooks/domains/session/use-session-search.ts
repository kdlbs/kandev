"use client";

import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { searchSessionMessages, type MessageSearchHit } from "@/lib/api/domains/session-api";

type SessionSearchState = {
  isOpen: boolean;
  query: string;
  hits: MessageSearchHit[];
  isSearching: boolean;
  activeHitId: string | null;
};

const DEBOUNCE_MS = 180;
const MAX_BACKFILL_ITERATIONS = 40;

export type SessionSearchHook = SessionSearchState & {
  open: () => void;
  close: () => void;
  setQuery: (q: string) => void;
  setActiveHit: (id: string | null) => void;
};

/** Query generation is captured before debounce and checked at every settlement. */
function useDebouncedSearch(
  sessionId: string | null | undefined,
  requestIdRef: React.RefObject<number>,
  setHits: (hits: MessageSearchHit[]) => void,
  setIsSearching: (v: boolean) => void,
  canSearch: () => boolean,
) {
  return useCallback(
    async (q: string, myId: number) => {
      const isCurrent = () => canSearch() && requestIdRef.current === myId;
      if (!sessionId || !isCurrent()) return;
      const trimmed = q.trim();
      if (!trimmed) return;
      setIsSearching(true);
      try {
        const resp = await searchSessionMessages(sessionId, trimmed, 50);
        if (!isCurrent()) return;
        setHits(resp.hits ?? []);
      } catch (err) {
        if (!isCurrent()) return;
        // i18n-exempt: request diagnostic logged to the console, never rendered as UI copy.
        console.error("Session search failed:", err);
        setHits([]);
      } finally {
        if (isCurrent()) setIsSearching(false);
      }
    },
    [sessionId, requestIdRef, setHits, setIsSearching, canSearch],
  );
}

/** Only committed session and search lifetimes can accept retained callbacks. */
function useSearchContext(
  sessionId: string | null | undefined,
  resetSearch: () => void,
  cancelSearch: () => void,
) {
  const session = useMemo(() => ({ sessionId }), [sessionId]);
  const committedSessionRef = useRef<typeof session | null>(null);
  const [queryLifetime, setQueryLifetime] = useState(() => Symbol());
  const committedQueryRef = useRef<symbol | null>(null);

  useLayoutEffect(() => {
    committedSessionRef.current = session;
    committedQueryRef.current = queryLifetime;
    resetSearch();
    return () => {
      committedSessionRef.current = null;
      committedQueryRef.current = null;
      cancelSearch();
    };
  }, [session, queryLifetime, resetSearch, cancelSearch]);

  const isCurrentSession = useCallback(() => committedSessionRef.current === session, [session]);
  const isCurrentQuery = useCallback(
    () => isCurrentSession() && committedQueryRef.current === queryLifetime,
    [isCurrentSession, queryLifetime],
  );
  const retireQuery = useCallback(() => {
    committedQueryRef.current = null;
    setQueryLifetime(Symbol());
  }, []);
  return { isCurrentSession, isCurrentQuery, retireQuery };
}

/** Focus a hit in the DOM with scroll + flash animation. */
function focusMessageElement(id: string, navigate?: (id: string) => HTMLElement | null): boolean {
  const el = navigate ? navigate(id) : document.getElementById(`msg-${id}`);
  if (!el) return false;
  if (!navigate) {
    // Without a navigation callback there is no guard against competing chat scrolling.
    el.scrollIntoView({ block: "center", behavior: "auto" });
  }
  el.classList.remove("search-flash");
  // Force reflow so animation replays when re-clicked
  void el.offsetWidth;
  el.classList.add("search-flash");
  window.setTimeout(() => el.classList.remove("search-flash"), 1400);
  return true;
}

/** setActiveHit + backfill loop with generation-based cancellation. */
function useSetActiveHit(
  loadOlder: (() => Promise<number>) | undefined,
  setActiveHitIdState: (id: string | null) => void,
  genRef: React.RefObject<number>,
  navigate?: (id: string) => HTMLElement | null,
) {
  return useCallback(
    async (id: string | null) => {
      setActiveHitIdState(id);
      if (!id) return;
      const myGen = ++genRef.current;
      if (focusMessageElement(id, navigate)) return;
      if (!loadOlder) return;
      for (let i = 0; i < MAX_BACKFILL_ITERATIONS; i++) {
        const loaded = await loadOlder();
        // Superseded by a newer setActiveHit, or close/unmount bumped genRef.
        if (genRef.current !== myGen) return;
        if (loaded === 0) break;
        if (focusMessageElement(id, navigate)) return;
      }
    },
    [loadOlder, setActiveHitIdState, genRef, navigate],
  );
}

export function useSessionSearch(
  sessionId: string | null | undefined,
  loadOlder?: () => Promise<number>,
  navigate?: (id: string) => HTMLElement | null,
): SessionSearchHook {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [hits, setHits] = useState<MessageSearchHit[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [activeHitId, setActiveHitIdState] = useState<string | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const requestIdRef = useRef(0);
  const isOpenRef = useRef(false);
  // Generation counter for setActiveHit — bumping it aborts any in-flight
  // backfill loop (new click, search bar close, or component unmount).
  const activeHitGenRef = useRef(0);

  const cancelSearch = useCallback(() => {
    requestIdRef.current++;
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    timeoutRef.current = null;
    activeHitGenRef.current++;
  }, []);
  const resetSearch = useCallback(() => {
    cancelSearch();
    setHits([]);
    setActiveHitIdState(null);
    setQueryState("");
    setIsSearching(false);
  }, [cancelSearch]);
  const { isCurrentSession, isCurrentQuery, retireQuery } = useSearchContext(
    sessionId,
    resetSearch,
    cancelSearch,
  );
  const canSearch = useCallback(() => isCurrentQuery() && isOpenRef.current, [isCurrentQuery]);
  const runSearch = useDebouncedSearch(sessionId, requestIdRef, setHits, setIsSearching, canSearch);

  const setQuery = useCallback(
    (q: string) => {
      if (!canSearch()) return;
      resetSearch();
      setQueryState(q);
      if (!sessionId || !q.trim()) return;
      const myId = requestIdRef.current;
      timeoutRef.current = setTimeout(() => runSearch(q, myId), DEBOUNCE_MS);
    },
    [canSearch, resetSearch, sessionId, runSearch],
  );

  const open = useCallback(() => {
    if (!isCurrentSession()) return;
    isOpenRef.current = true;
    setIsOpen(true);
  }, [isCurrentSession]);
  const close = useCallback(() => {
    if (!isCurrentSession()) return;
    isOpenRef.current = false;
    resetSearch();
    retireQuery();
    setIsOpen(false);
  }, [isCurrentSession, resetSearch, retireQuery]);
  const selectHit = useSetActiveHit(loadOlder, setActiveHitIdState, activeHitGenRef, navigate);
  const setActiveHit = useCallback(
    (id: string | null) => {
      if (isCurrentSession()) return selectHit(id);
    },
    [isCurrentSession, selectHit],
  );

  return {
    isOpen,
    query,
    hits,
    isSearching,
    activeHitId,
    open,
    close,
    setQuery,
    setActiveHit,
  };
}
