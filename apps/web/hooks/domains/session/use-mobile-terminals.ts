"use client";

import { useEffect, useRef } from "react";
import { useTerminals } from "./use-terminals";
import { BOTTOM_PANEL_TERMINAL_ID } from "./use-terminals-build";
import { useUserShells } from "./use-user-shells";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { userShellScopeKey } from "@/lib/state/user-shell-scope";

/**
 * Module-level guard for the auto-create-first-shell effect, keyed by
 * `environmentId:taskID`. `useMobileTerminals` is called from several places
 * that all end up rendering the same terminal pane (the pane itself, the
 * picker pill, the picker sheet content) — a per-instance ref guard would
 * let each one fire `createUserShell` on first mount, racing into multiple
 * shells. The Set is shared across instances so only the first effect to
 * run for a given (env, task) pair triggers creation.
 *
 * Why env+task: the auto-create must wait for the real taskID — otherwise
 * we'd auto-create a non-persistent legacy shell against an env, mark the
 * env as "done" in the guard, and the real task's first-class terminal
 * would never get created.
 */
const autoCreatedScopes = new Set<string>();

function autoCreateScope(environmentId: string, taskID: string | null): string | null {
  if (!taskID) return null;
  return `${environmentId}:${taskID}`;
}

/**
 * Release the auto-create guard so the next mount (or shell-list change)
 * re-runs the auto-create effect. Call this when the user explicitly closes
 * the last terminal in an env — otherwise the pane shows "Starting
 * terminal…" forever because the guard prevents recreation. We clear every
 * scope keyed on this env id regardless of task, so any subsequent re-mount
 * finds a clean slate.
 */
export function releaseAutoCreatedEnvironment(environmentId: string): void {
  for (const scope of Array.from(autoCreatedScopes)) {
    if (scope.startsWith(`${environmentId}:`)) {
      autoCreatedScopes.delete(scope);
    }
  }
}

/** Test-only: reset the module-level guard so each test starts from scratch. */
export function __resetAutoCreatedEnvironmentsForTest(): void {
  autoCreatedScopes.clear();
}

/**
 * Mobile wrapper around `useTerminals`. Auto-creates a first shell when the
 * server-side shell list loads empty so the user always sees one terminal
 * mounted by default. Returns the same interface as `useTerminals`.
 */
export function useMobileTerminals(sessionId: string | null) {
  const store = useAppStoreApi();
  const environmentId = useAppStore((s) =>
    sessionId ? (s.environmentIdBySessionId[sessionId] ?? null) : null,
  );
  const taskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const result = useTerminals({ sessionId, environmentId });
  // Read user-shell loaded flag directly so the auto-create trigger has
  // primitive dependencies — depending on `result` would re-run on every
  // render and would also race against React's effect ordering.
  // taskID flows in so the backend's DB-backed ordinary-shell path
  // returns parked + open ordinary rows (the orphan filter drops them
  // otherwise).
  const {
    isLoaded: shellsLoaded,
    isLoading,
    hasSuccessfulList,
    shells,
  } = useUserShells(environmentId, taskID);
  // The phone pane renders visible tabs only; bottom-panel and parked rows do
  // not satisfy the "one usable terminal" invariant.
  const hasVisibleShell = shells.some(
    (shell) => shell.state !== "parked" && shell.terminalId !== BOTTOM_PANEL_TERMINAL_ID,
  );
  const addTerminalRef = useRef(result.addTerminal);
  useEffect(() => {
    addTerminalRef.current = result.addTerminal;
  }, [result.addTerminal]);

  useEffect(() => {
    // Creation authority comes from a successful current read, never from a
    // cached success while a new read is pending or from a settled failure.
    if (!environmentId || !shellsLoaded || isLoading || !hasSuccessfulList) return;
    const scope = autoCreateScope(environmentId, taskID);
    // Wait for a real taskID before auto-creating; otherwise the first
    // mount on a session without a hydrated active task would bootstrap a
    // legacy non-persistent shell and mark the env as "done" — pinning
    // future task-scoped mounts on a stale legacy entry.
    if (!scope) return;
    if (autoCreatedScopes.has(scope)) return;
    if (hasVisibleShell) {
      autoCreatedScopes.add(scope);
      return;
    }
    // Recheck authority at the mutation boundary so a read that started
    // between render and this effect cannot authorize a duplicate create.
    const key = userShellScopeKey({ environmentId, taskId: taskID });
    const shellState = store.getState().userShells;
    if (shellState.loading[key] || !shellState.listed[key]) return;
    autoCreatedScopes.add(scope);
    // Reset the guard if creation fails so the user gets a retry on the next
    // render cycle (e.g. after the WS reconnects). `addTerminal` returns void
    // but its inner promise can still reject; guard defensively.
    const promise = addTerminalRef.current() as unknown;
    if (promise && typeof (promise as Promise<unknown>).catch === "function") {
      (promise as Promise<unknown>).catch(() => {
        autoCreatedScopes.delete(scope);
      });
    }
  }, [environmentId, taskID, store, shellsLoaded, isLoading, hasSuccessfulList, hasVisibleShell]);

  return { ...result, environmentId };
}
