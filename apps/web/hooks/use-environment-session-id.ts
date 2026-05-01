import { useCallback, useRef } from "react";
import { useAppStore } from "@/components/state-provider";

type StoreSlice = {
  tasks: { activeSessionId: string | null };
  environmentIdBySessionId: Record<string, string>;
};

/**
 * Return a sessionId that only changes when the underlying environment changes.
 *
 * Some workspace readiness APIs still need a session lookup handle. This hook
 * keeps that handle stable across same-environment tab switches so downstream
 * components don't needlessly reconnect or re-fetch.
 */
export function useEnvironmentSessionId(): string | null {
  const cacheRef = useRef({ envId: null as string | null, sessionId: null as string | null });
  const selector = useCallback((state: StoreSlice) => {
    const sid = state.tasks.activeSessionId;
    const envId = sid ? (state.environmentIdBySessionId[sid] ?? sid) : null;
    if (envId === cacheRef.current.envId) return cacheRef.current.sessionId;
    cacheRef.current = { envId, sessionId: sid };
    return sid;
  }, []);
  return useAppStore(selector);
}

/** Return the active task environment ID without falling back to session ID. */
export function useEnvironmentId(): string | null {
  return useAppStore((state: StoreSlice) => {
    const sid = state.tasks.activeSessionId;
    return sid ? (state.environmentIdBySessionId[sid] ?? null) : null;
  });
}
