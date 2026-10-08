"use client";

import { createContext, useContext, useRef, type ReactNode } from "react";
import type { Message, TaskSession } from "@/lib/types/http";
import {
  selectActiveSessionRecovery,
  type ActiveSessionRecovery,
} from "@/lib/active-session-recovery";
import { useTaskLaunchErrorContext } from "../task-launch-error-context";
import { usePendingSessionRecovery } from "@/hooks/domains/session/session-recovery-pending";
import { matchingAutomaticRecovery } from "@/lib/session-recovery-presentation";

type RecoveryContext = {
  sessionId: string;
  model: ActiveSessionRecovery | null;
  pending: import("@/hooks/domains/session/use-session-recovery-actions").SessionRecoveryBusyAction;
};
const Context = createContext<RecoveryContext | null>(null);

export function SessionRecoveryProvider({
  session,
  messages,
  taskId,
  enabled,
  messagesLoading = false,
  children,
}: {
  session: TaskSession | null | undefined;
  messages: Message[];
  taskId: string | null;
  enabled: boolean;
  messagesLoading?: boolean;
  children: ReactNode;
}) {
  const pending = usePendingSessionRecovery(`${taskId ?? ""}\u0000${session?.id ?? ""}`);
  const lastPending = useRef(pending);
  if (pending) lastPending.current = pending;
  const previous = useRef<ActiveSessionRecovery | null>(null);
  const selected = useCurrentRecovery(session, messages, enabled);
  const launchErrorContext = useTaskLaunchErrorContext();
  const automaticRecovery = matchingAutomaticRecovery(
    launchErrorContext?.automaticRecovery,
    taskId,
    session?.id,
  );
  const retainPending = retainRecovery(enabled, session, previous.current);
  const requestLocalModel = requestLocalRecoveryModel(session?.id, automaticRecovery);
  const model = chooseRecoveryModel(selected, retainPending, previous.current, requestLocalModel);
  previous.current = model;
  const presentedModel = model ? { ...model, loading: messagesLoading } : null;
  return (
    <Context.Provider
      value={recoveryContextValue({
        enabled,
        session,
        model: presentedModel,
        pending,
        lastPending: lastPending.current,
      })}
    >
      {children}
    </Context.Provider>
  );
}

function chooseRecoveryModel(
  selected: ActiveSessionRecovery | null,
  retainPending: boolean,
  previous: ActiveSessionRecovery | null,
  requestLocal: ActiveSessionRecovery | null,
) {
  return selected ?? (retainPending ? previous : null) ?? requestLocal;
}

function recoveryContextValue({
  enabled,
  session,
  model,
  pending,
  lastPending,
}: {
  enabled: boolean;
  session: TaskSession | null | undefined;
  model: ActiveSessionRecovery | null;
  pending: RecoveryContext["pending"];
  lastPending: RecoveryContext["pending"];
}): RecoveryContext | null {
  if (!enabled || !session) return null;
  const startingPending = session.state === "STARTING" && model ? (lastPending ?? "resume") : null;
  return {
    sessionId: session.id,
    model,
    pending: pending ?? startingPending,
  };
}

function requestLocalRecoveryModel(
  sessionId: string | undefined,
  recovery: ReturnType<typeof matchingAutomaticRecovery>,
): ActiveSessionRecovery | null {
  if (!sessionId) return null;
  if (recovery?.noticeKind === "inspection_busy") {
    return { sessionId, kind: "recovery_inspection_busy", summary: recovery.notice ?? undefined };
  }
  if (recovery?.error) return { sessionId, kind: "generic", summary: recovery.error };
  return null;
}

export function useSessionComposerRecovery(sessionId?: string | null) {
  const context = useContext(Context);
  return context?.sessionId === sessionId ? context : null;
}

function retainRecovery(
  enabled: boolean,
  session: TaskSession | null | undefined,
  previous: ActiveSessionRecovery | null,
) {
  return enabled && session?.state === "STARTING" && previous?.sessionId === session.id;
}

function useCurrentRecovery(
  session: TaskSession | null | undefined,
  messages: Message[],
  enabled: boolean,
) {
  const taskError = useTaskLaunchErrorContext()?.statusSummary?.active_error;
  return enabled ? selectActiveSessionRecovery(session, messages, taskError) : null;
}
