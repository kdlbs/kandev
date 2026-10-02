import type { StoreApi } from "zustand";
import type { WebSocketClient } from "@/lib/ws/client";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { hasRecoverableGitStatusDetailFailure } from "@/lib/state/slices/session-runtime/git-status-detail-failures";
import type { AppState } from "@/lib/state/store";

export type GitRefreshScopeContext = {
  client: WebSocketClient;
  store: StoreApi<AppState>;
  sessionId: string;
  environmentId: string;
  scopeKey: string;
  generation: number;
};
export type GitRefreshOwnerContext = Pick<GitRefreshScopeContext, "sessionId" | "store">;

type RecoveryTimer = { handle: ReturnType<typeof setTimeout> };

const recoveryTimers = new Map<string, RecoveryTimer>();
const recoveryFailureCounts = new Map<string, number>();
const RECOVERY_DELAYS_MS = [5_000, 10_000, 20_000, 30_000] as const;

export function clearRecoveryTimer(scopeKey: string) {
  const timer = recoveryTimers.get(scopeKey);
  if (timer) clearTimeout(timer.handle);
  recoveryTimers.delete(scopeKey);
}

export function clearRecoveryState(scopeKey: string) {
  clearRecoveryTimer(scopeKey);
  recoveryFailureCounts.delete(scopeKey);
}

export function clearRecoveryTimersForClient(clientId: number) {
  for (const scopeKey of recoveryTimers.keys()) {
    if (scopeKey.startsWith(`${clientId}\u0000`)) clearRecoveryTimer(scopeKey);
  }
}

export function scheduleRecoveryTimer(scopeKey: string, callback: () => void) {
  if (recoveryTimers.has(scopeKey)) return;
  const failureCount = recoveryFailureCounts.get(scopeKey) ?? 0;
  const delay = RECOVERY_DELAYS_MS[Math.min(failureCount, RECOVERY_DELAYS_MS.length - 1)];
  recoveryFailureCounts.set(scopeKey, failureCount + 1);
  const timer: RecoveryTimer = {
    handle: setTimeout(() => {
      if (recoveryTimers.get(scopeKey) !== timer) return;
      recoveryTimers.delete(scopeKey);
      callback();
    }, delay),
  };
  recoveryTimers.set(scopeKey, timer);
}

export function isBrowserForeground(): boolean {
  if (typeof document === "undefined") return true;
  return document.visibilityState === "visible" && document.hasFocus();
}

export function currentEnvironmentMatches(
  store: StoreApi<AppState>,
  sessionId: string,
  environmentId: string,
): boolean {
  return (store.getState().environmentIdBySessionId[sessionId] ?? sessionId) === environmentId;
}

export function recoveryContextForScope(
  context: GitRefreshScopeContext,
  generation: number,
  registeredOwners?: Map<symbol, GitRefreshOwnerContext>,
): GitRefreshScopeContext | null {
  if (registeredOwners) {
    for (const owner of registeredOwners.values()) {
      if (currentEnvironmentMatches(owner.store, owner.sessionId, context.environmentId)) {
        return { ...context, ...owner, generation };
      }
    }
  }
  if (
    context.generation === generation &&
    currentEnvironmentMatches(context.store, context.sessionId, context.environmentId)
  ) {
    return context;
  }
  return null;
}

export function hasActiveAttemptForScope(
  attempts: Iterable<[string, { controller: AbortController }]>,
  scopeKey: string,
  generation: number,
): boolean {
  const prefix = `${scopeKey}\u0000${generation}\u0000`;
  for (const [key, attempt] of attempts) {
    if (key.startsWith(prefix) && !attempt.controller.signal.aborted) return true;
  }
  return false;
}

export function hasRecoverableReadFailure(
  store: StoreApi<AppState>,
  environmentId: string,
): boolean {
  const gitStatus = store.getState().gitStatus;
  const statuses = [
    ...Object.values(gitStatus.byEnvironmentRepo[environmentId] ?? {}),
    gitStatus.byEnvironmentId[environmentId],
  ].filter((status): status is GitStatusEntry => Boolean(status));
  const refreshes = [
    gitStatus.refreshByEnvironmentId?.[environmentId],
    ...Object.values(gitStatus.refreshByEnvironmentRepo?.[environmentId] ?? {}),
  ];
  if (refreshes.some((refresh) => refresh?.state === "unavailable")) return true;
  return statuses.some((status) => {
    return status.status_state === "unavailable" || hasRecoverableGitStatusDetailFailure(status);
  });
}
