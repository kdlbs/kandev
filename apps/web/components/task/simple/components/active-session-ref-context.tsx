"use client";

import { createContext, useCallback, useContext, useRef, type ReactNode } from "react";

type ActiveSessionRefContextValue = {
  /** Register the DOM node for the active session's timeline entry. */
  setActiveRef: (sessionId: string, node: HTMLElement | null) => void;
  /** Read the currently-registered active session DOM node, if any. */
  getActiveNode: () => HTMLElement | null;
};

const ActiveSessionRefContext = createContext<ActiveSessionRefContextValue | null>(null);

/**
 * Scopes a "which DOM node is the currently-active session entry" registry
 * to the task detail page. The topbar Working spinner uses this to scroll
 * the active entry into view when clicked. Avoids growing the global zustand
 * store with what is purely page-local UI plumbing.
 */
export function ActiveSessionRefProvider({ children }: { children: ReactNode }) {
  const activeIdRef = useRef<string | null>(null);
  const activeNodeRef = useRef<HTMLElement | null>(null);

  const setActiveRef = useCallback((sessionId: string, node: HTMLElement | null) => {
    if (node) {
      activeIdRef.current = sessionId;
      activeNodeRef.current = node;
      return;
    }
    // Only clear if the unmount/null comes from the currently-active session.
    if (activeIdRef.current === sessionId) {
      activeIdRef.current = null;
      activeNodeRef.current = null;
    }
  }, []);

  const getActiveNode = useCallback(() => activeNodeRef.current, []);

  return (
    <ActiveSessionRefContext.Provider value={{ setActiveRef, getActiveNode }}>
      {children}
    </ActiveSessionRefContext.Provider>
  );
}

export function useActiveSessionRef(): ActiveSessionRefContextValue {
  const ctx = useContext(ActiveSessionRefContext);
  if (!ctx) {
    return {
      setActiveRef: () => {},
      getActiveNode: () => null,
    };
  }
  return ctx;
}
