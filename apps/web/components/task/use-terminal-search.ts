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

export function useTerminalSearch({
  xtermRef,
  isTerminalReady,
}: UseTerminalSearchOptions): TerminalSearchState {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [regex, setRegex] = useState(false);
  const [matchInfo, setMatchInfo] = useState({ current: 0, total: 0 });

  const addonRef = useRef<SearchAddon | null>(null);

  useEffect(() => {
    const term = xtermRef.current;
    if (!isTerminalReady || !term || addonRef.current) return;
    let cancelled = false;
    let disposables: { dispose: () => void } | null = null;
    let loaded: SearchAddon | null = null;
    // Dynamic import so @xterm/addon-search (UMD bundle touching `self`) does
    // not evaluate during SSR.
    import("@xterm/addon-search")
      .then(({ SearchAddon }) => {
        if (cancelled || addonRef.current) return;
        const addon = new SearchAddon();
        try {
          term.loadAddon(addon);
        } catch {
          return;
        }
        addonRef.current = addon;
        loaded = addon;
        disposables = addon.onDidChangeResults(({ resultIndex, resultCount }) => {
          setMatchInfo({
            current: resultIndex >= 0 ? resultIndex + 1 : 0,
            total: resultCount,
          });
        });
      })
      .catch(() => {
        /* addon failed to load; search simply remains disabled */
      });
    return () => {
      cancelled = true;
      disposables?.dispose();
      loaded?.dispose();
      addonRef.current = null;
    };
  }, [xtermRef, isTerminalReady]);

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
    if (!addonRef.current || !query || hasError) return;
    addonRef.current.findNext(query, { ...buildOptions(), incremental: true });
  }, [query, hasError, buildOptions]);

  const findPrev = useCallback(() => {
    if (!addonRef.current || !query || hasError) return;
    addonRef.current.findPrevious(query, buildOptions());
  }, [query, hasError, buildOptions]);

  useEffect(() => {
    if (!isOpen) return;
    if (!query) {
      addonRef.current?.clearDecorations();
      // clearDecorations does not fire onDidChangeResults; reset counter manually.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setMatchInfo({ current: 0, total: 0 });
      return;
    }
    if (hasError) return;
    addonRef.current?.findNext(query, { ...buildOptions(), incremental: true });
  }, [isOpen, query, caseSensitive, regex, hasError, buildOptions]);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => {
    setIsOpen(false);
    addonRef.current?.clearDecorations();
  }, []);

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
