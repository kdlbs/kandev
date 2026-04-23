"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Terminal } from "@xterm/xterm";
import type { SearchAddon } from "@xterm/addon-search";

type UseTerminalSearchOptions = {
  xtermRef: React.RefObject<Terminal | null>;
  /** Resolves once the terminal instance has been created. */
  isTerminalReady: boolean;
};

export type TerminalSearchState = {
  isOpen: boolean;
  open: () => void;
  close: () => void;
  query: string;
  setQuery: (value: string) => void;
  caseSensitive: boolean;
  setCaseSensitive: (value: boolean) => void;
  regex: boolean;
  setRegex: (value: boolean) => void;
  findNext: () => void;
  findPrev: () => void;
  matchInfo: { current: number; total: number };
  hasError: boolean;
  errorText: string | undefined;
};

function isValidRegex(pattern: string): boolean {
  try {
    new RegExp(pattern);
    return true;
  } catch {
    return false;
  }
}

/**
 * Load the SearchAddon when the terminal becomes ready.
 * Tracks addon readiness via state so effects waiting on the addon re-run
 * after the dynamic import resolves (avoids a race where the user's query
 * is silently dropped because addonRef was still null).
 */
function useSearchAddon(
  xtermRef: React.RefObject<Terminal | null>,
  isTerminalReady: boolean,
  onResults: (info: { current: number; total: number }) => void,
): { addon: SearchAddon | null } {
  const [addon, setAddon] = useState<SearchAddon | null>(null);
  const onResultsRef = useRef(onResults);
  useEffect(() => {
    onResultsRef.current = onResults;
  });

  useEffect(() => {
    const term = xtermRef.current;
    if (!isTerminalReady || !term) return;
    let cancelled = false;
    let loaded: SearchAddon | null = null;
    let resultsSub: { dispose: () => void } | null = null;
    // Dynamic import keeps the UMD bundle (which touches `self`) out of SSR.
    import("@xterm/addon-search")
      .then(({ SearchAddon }) => {
        if (cancelled) return;
        const a = new SearchAddon();
        try {
          term.loadAddon(a);
        } catch {
          return;
        }
        loaded = a;
        resultsSub = a.onDidChangeResults(({ resultIndex, resultCount }) => {
          onResultsRef.current({
            current: resultIndex >= 0 ? resultIndex + 1 : 0,
            total: resultCount,
          });
        });
        setAddon(a);
      })
      .catch(() => {
        /* addon failed to load; search remains disabled */
      });
    return () => {
      cancelled = true;
      resultsSub?.dispose();
      loaded?.dispose();
      setAddon(null);
    };
  }, [xtermRef, isTerminalReady]);

  return { addon };
}

export function useTerminalSearch({
  xtermRef,
  isTerminalReady,
}: UseTerminalSearchOptions): TerminalSearchState {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [regex, setRegex] = useState(false);
  const [matchInfo, setMatchInfo] = useState({ current: 0, total: 0 });

  const handleResults = useCallback(
    (info: { current: number; total: number }) =>
      setMatchInfo((prev) =>
        prev.current === info.current && prev.total === info.total ? prev : info,
      ),
    [],
  );
  const { addon } = useSearchAddon(xtermRef, isTerminalReady, handleResults);

  const hasError = regex && query.length > 0 && !isValidRegex(query);
  const errorText = hasError ? "Invalid regex" : undefined;

  const buildOptions = useCallback(
    () => ({
      regex,
      caseSensitive,
      decorations: {
        matchBackground: "#facc15",
        matchBorder: "#eab308",
        matchOverviewRuler: "#eab308",
        activeMatchBackground: "#fb923c",
        activeMatchBorder: "#ea580c",
        activeMatchColorOverviewRuler: "#ea580c",
      },
    }),
    [regex, caseSensitive],
  );

  const findNext = useCallback(() => {
    if (!addon || !query || hasError) return;
    addon.findNext(query, { ...buildOptions(), incremental: true });
  }, [addon, query, hasError, buildOptions]);

  const findPrev = useCallback(() => {
    if (!addon || !query || hasError) return;
    addon.findPrevious(query, buildOptions());
  }, [addon, query, hasError, buildOptions]);

  useEffect(() => {
    if (!isOpen || !addon) return;
    if (!query) {
      addon.clearDecorations();
      // clearDecorations does not fire onDidChangeResults; reset counter manually.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setMatchInfo({ current: 0, total: 0 });
      return;
    }
    if (hasError) return;
    addon.findNext(query, { ...buildOptions(), incremental: true });
  }, [isOpen, query, caseSensitive, regex, hasError, buildOptions, addon]);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => {
    setIsOpen(false);
    addon?.clearDecorations();
  }, [addon]);

  return {
    isOpen,
    open,
    close,
    query,
    setQuery,
    caseSensitive,
    setCaseSensitive,
    regex,
    setRegex,
    findNext,
    findPrev,
    matchInfo,
    hasError,
    errorText,
  };
}
