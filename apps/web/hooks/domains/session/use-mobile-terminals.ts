"use client";

import { useEffect, useRef } from "react";
import { useTerminals } from "./use-terminals";
import { useUserShells } from "./use-user-shells";
import { useAppStore } from "@/components/state-provider";

/**
 * Mobile wrapper around `useTerminals`. Auto-creates a first shell when the
 * server-side shell list loads empty so the user always sees one terminal
 * mounted by default. Returns the same interface as `useTerminals`.
 */
export function useMobileTerminals(sessionId: string | null) {
  const environmentId = useAppStore((s) =>
    sessionId ? (s.environmentIdBySessionId[sessionId] ?? null) : null,
  );
  const result = useTerminals({ sessionId, environmentId });
  // Read user-shell loaded flag directly so the auto-create trigger has a
  // primitive dependency (depending on the result object would re-run the
  // effect every render and continuously cancel the timer).
  const { isLoaded: shellsLoaded, shells } = useUserShells(environmentId);
  const addTerminalRef = useRef(result.addTerminal);
  useEffect(() => {
    addTerminalRef.current = result.addTerminal;
  }, [result.addTerminal]);
  const autoCreatedRef = useRef<string | null>(null);

  useEffect(() => {
    if (!environmentId || !shellsLoaded) return;
    if (autoCreatedRef.current === environmentId) return;
    if (shells.length > 0) {
      autoCreatedRef.current = environmentId;
      return;
    }
    autoCreatedRef.current = environmentId;
    // Reset the guard if creation fails so the user gets a retry on the next
    // render cycle (e.g. after the WS reconnects). addTerminal returns void
    // but its inner promise can still reject; guard defensively.
    const result = addTerminalRef.current() as unknown;
    if (result && typeof (result as Promise<unknown>).catch === "function") {
      (result as Promise<unknown>).catch(() => {
        if (autoCreatedRef.current === environmentId) {
          autoCreatedRef.current = null;
        }
      });
    }
  }, [environmentId, shellsLoaded, shells.length]);

  return { ...result, environmentId };
}
