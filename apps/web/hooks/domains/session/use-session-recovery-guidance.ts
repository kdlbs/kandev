import { useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { useStore } from "zustand";
import { useOptionalAppStoreApi } from "@/components/state-provider";
import type { TaskSession } from "@/lib/types/http";
import {
  getSessionRecoveryGuidanceStore,
  invalidateSessionRecoveryGuidance,
  isSessionRecoveryGuidanceCurrent,
  reconcileSessionRecoveryGuidanceProjection,
  sessionRecoveryGuidanceIdentityFromSession,
  sessionRecoveryGuidanceKey,
  type SessionRecoveryGuidanceIdentity,
} from "@/lib/session/session-recovery-guidance";

const fallbackOwner = {};

/** Shares validated, incident-bound continuation guidance within one app store. */
export function useSessionRecoveryGuidance(
  taskId: string | null,
  sessionId: string | null,
  session: TaskSession | null | undefined,
  errorStamp?: string | null,
) {
  const owner = useOptionalAppStoreApi() ?? fallbackOwner;
  const identity = useMemo<SessionRecoveryGuidanceIdentity>(
    () => createIdentity(taskId, sessionId, session, errorStamp),
    [errorStamp, session, sessionId, taskId],
  );
  const identityRef = useRef(identity);
  useLayoutEffect(() => {
    identityRef.current = identity;
  }, [identity]);
  const store = getSessionRecoveryGuidanceStore(owner);
  const key = taskId && sessionId ? sessionRecoveryGuidanceKey(taskId, sessionId) : null;
  const entry = useStore(store, (state) => {
    if (!key || !session) return null;
    const candidate = state.entries.get(key);
    return candidate && isSessionRecoveryGuidanceCurrent(candidate, identity) ? candidate : null;
  });

  useEffect(() => {
    if (!taskId || !sessionId) return;
    if (!session) {
      invalidateSessionRecoveryGuidance(owner, taskId, sessionId);
      return;
    }
    reconcileSessionRecoveryGuidanceProjection(owner, identity);
  }, [identity, key, owner, session, sessionId, store, taskId]);

  return { owner, identity, identityRef, entry };
}

function createIdentity(
  taskId: string | null,
  sessionId: string | null,
  session: TaskSession | null | undefined,
  errorStamp?: string | null,
): SessionRecoveryGuidanceIdentity {
  if (taskId && sessionId && session?.id === sessionId && session.task_id === taskId) {
    return sessionRecoveryGuidanceIdentityFromSession(taskId, session, errorStamp);
  }
  return {
    taskId: taskId ?? "",
    sessionId: sessionId ?? "",
    requestKey: `${taskId ?? ""}\u0000${sessionId ?? ""}\u0000${errorStamp ?? ""}`,
    errorStamp,
  };
}
