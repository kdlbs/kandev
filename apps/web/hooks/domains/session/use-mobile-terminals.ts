"use client";

import { useEffect, useRef } from "react";
import { useTerminals } from "./use-terminals";
import { useUserShells } from "./use-user-shells";
import { useAppStore } from "@/components/state-provider";

/**
 * Module-level guard for the auto-create-first-shell effect, keyed by
 * environmentId. `useMobileTerminals` is called from several places that all
 * end up rendering the same terminal pane (the pane itself, the picker pill,
 * the picker sheet content) — a per-instance ref guard would let each one
 * fire `createUserShell` on first mount, racing into multiple shells. The
 * Set is shared across instances so only the first effect to run for a given
 * environment triggers creation.
 */
const autoCreatedEnvironments = new Set<string>();

/**
 * Release the auto-create guard for a specific environment so the next mount
 * (or shell-list change) re-runs the auto-create effect. Call this when the
 * user explicitly closes the last terminal in an env — otherwise the pane
 * shows "Starting terminal…" forever because the guard prevents recreation.
 */
export function releaseAutoCreatedEnvironment(environmentId: string): void {
  autoCreatedEnvironments.delete(environmentId);
}

/** Test-only: reset the module-level guard so each test starts from scratch. */
export function __resetAutoCreatedEnvironmentsForTest(): void {
  autoCreatedEnvironments.clear();
}

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
  // Read user-shell loaded flag directly so the auto-create trigger has
  // primitive dependencies — depending on `result` would re-run on every
  // render and would also race against React's effect ordering.
  const { isLoaded: shellsLoaded, shells } = useUserShells(environmentId);
  const addTerminalRef = useRef(result.addTerminal);
  useEffect(() => {
    addTerminalRef.current = result.addTerminal;
  }, [result.addTerminal]);

  useEffect(() => {
    if (!environmentId || !shellsLoaded) return;
    if (autoCreatedEnvironments.has(environmentId)) return;
    if (shells.length > 0) {
      autoCreatedEnvironments.add(environmentId);
      return;
    }
    autoCreatedEnvironments.add(environmentId);
    // Reset the guard if creation fails so the user gets a retry on the next
    // render cycle (e.g. after the WS reconnects). `addTerminal` returns void
    // but its inner promise can still reject; guard defensively.
    const promise = addTerminalRef.current() as unknown;
    if (promise && typeof (promise as Promise<unknown>).catch === "function") {
      (promise as Promise<unknown>).catch(() => {
        autoCreatedEnvironments.delete(environmentId);
      });
    }
  }, [environmentId, shellsLoaded, shells.length]);

  return { ...result, environmentId };
}
