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

export function useSessionSearch(
  sessionId: string | null | undefined,
  loadOlder?: () => Promise<number>,
): SessionSearchHook {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [hits, setHits] = useState<MessageSearchHit[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [activeHitId, setActiveHitIdState] = useState<string | null>(null);
  const requestIdRef = useRef(0);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Generation counter for setActiveHit — lets an in-flight backfill loop
  // bail out if the user clicks a newer hit while the old loop is awaiting
  // loadOlder(), preventing a stale scroll/flash from overriding the user.
  const activeHitGenRef = useRef(0);

  const runSearch = useCallback(
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
    [sessionId],
  );

  const setQuery = useCallback(
    (q: string) => {
      setQueryState(q);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = setTimeout(() => runSearch(q), DEBOUNCE_MS);
    },
    [runSearch],
  );

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  useEffect(() => {
    if (!isOpen) {
      setHits([]);
      setActiveHitIdState(null);
      setQueryState("");
      setIsSearching(false);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    }
  }, [isOpen]);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => setIsOpen(false), []);

  const setActiveHit = useCallback(
    async (id: string | null) => {
      setActiveHitIdState(id);
      if (!id) return;
      const myGen = ++activeHitGenRef.current;
      const tryFocus = (): boolean => {
        const el = document.getElementById(`msg-${id}`);
        if (!el) return false;
        el.scrollIntoView({ block: "center", behavior: "smooth" });
        el.classList.remove("search-flash");
        // Force reflow so animation replays when re-clicked
        void el.offsetWidth;
        el.classList.add("search-flash");
        window.setTimeout(() => el.classList.remove("search-flash"), 1400);
        return true;
      };
      if (tryFocus()) return;
      if (!loadOlder) return;
      for (let i = 0; i < MAX_BACKFILL_ITERATIONS; i++) {
        const loaded = await loadOlder();
        // Superseded by a newer setActiveHit call (user clicked another hit).
        if (activeHitGenRef.current !== myGen) return;
        if (loaded === 0) break;
        if (tryFocus()) return;
      }
    },
    [loadOlder],
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
