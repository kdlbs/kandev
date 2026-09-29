import { useSyncExternalStore } from "react";
import type { SessionRecoveryBusyAction } from "./use-session-recovery-actions";

const pending = new Map<string, { action: SessionRecoveryBusyAction; token: symbol }>();
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((listener) => listener());
const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/** Admission is synchronous so repeated taps and separate consumers share one operation. */
export function claimSessionRecovery(key: string, action: SessionRecoveryBusyAction) {
  if (pending.has(key)) return null;
  const token = Symbol();
  pending.set(key, { action, token });
  notify();
  return () => {
    if (pending.get(key)?.token !== token) return;
    pending.delete(key);
    notify();
  };
}

export function usePendingSessionRecovery(key: string) {
  return useSyncExternalStore(
    subscribe,
    () => pending.get(key)?.action ?? null,
    () => null,
  );
}
