"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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

/** Debounced fetch + request-ID cancellation. */
function useDebouncedSearch(
  sessionId: string | null | undefined,
  setHits: (hits: MessageSearchHit[]) => void,
  setIsSearching: (v: boolean) => void,
) {
  const requestIdRef = useRef(0);
  return useCallback(
    async (q: string) => {
      if (!sessionId) return;
      const trimmed = q.trim();
      if (!trimmed) {
        setHits([]);
        setIsSearching(false);
        return;
      }
      const myId = ++requestIdRef.current;
      setIsSearching(true);
      try {
        const resp = await searchSessionMessages(sessionId, trimmed, 50);
        if (requestIdRef.current !== myId) return;
        setHits(resp.hits ?? []);
      } catch (err) {
        if (requestIdRef.current !== myId) return;
        console.error("Session search failed:", err);
        setHits([]);
      } finally {
        if (requestIdRef.current === myId) setIsSearching(false);
      }
    },
    [sessionId, setHits, setIsSearching],
  );
}

/** Focus a hit in the DOM with scroll + flash animation. */
function focusMessageElement(id: string): boolean {
  const el = document.getElementById(`msg-${id}`);
  if (!el) return false;
  el.scrollIntoView({ block: "center", behavior: "smooth" });
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
) {
  return useCallback(
    async (id: string | null) => {
      setActiveHitIdState(id);
      if (!id) return;
      const myGen = ++genRef.current;
      if (focusMessageElement(id)) return;
      if (!loadOlder) return;
      for (let i = 0; i < MAX_BACKFILL_ITERATIONS; i++) {
        const loaded = await loadOlder();
        // Superseded by a newer setActiveHit, or close/unmount bumped genRef.
        if (genRef.current !== myGen) return;
        if (loaded === 0) break;
        if (focusMessageElement(id)) return;
      }
    },
    [loadOlder, setActiveHitIdState, genRef],
  );
}

export function useSessionSearch(
  sessionId: string | null | undefined,
  loadOlder?: () => Promise<number>,
): SessionSearchHook {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [hits, setHits] = useState<MessageSearchHit[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [activeHitId, setActiveHitIdState] = useState<string | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Generation counter for setActiveHit — bumping it aborts any in-flight
  // backfill loop (new click, search bar close, or component unmount).
  const activeHitGenRef = useRef(0);

  const runSearch = useDebouncedSearch(sessionId, setHits, setIsSearching);

  const setQuery = useCallback(
    (q: string) => {
      setQueryState(q);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = setTimeout(() => runSearch(q), DEBOUNCE_MS);
    },
    [runSearch],
  );

  useEffect(() => {
    const genRef = activeHitGenRef;
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      // Abort any in-flight setActiveHit backfill loop on unmount.
      genRef.current++;
    };
  }, []);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => {
    setIsOpen(false);
    setHits([]);
    setActiveHitIdState(null);
    setQueryState("");
    setIsSearching(false);
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    // Abort any in-flight setActiveHit backfill loop.
    activeHitGenRef.current++;
  }, []);
  const setActiveHit = useSetActiveHit(loadOlder, setActiveHitIdState, activeHitGenRef);

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
