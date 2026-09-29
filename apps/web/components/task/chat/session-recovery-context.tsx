"use client";

import { createContext, useContext, useRef, type ReactNode } from "react";
import type { Message, TaskSession } from "@/lib/types/http";
import {
  selectActiveSessionRecovery,
  type ActiveSessionRecovery,
} from "@/lib/active-session-recovery";
import { useTaskLaunchErrorContext } from "../task-launch-error-context";
import { usePendingSessionRecovery } from "@/hooks/domains/session/session-recovery-pending";

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
  const retainPending = retainRecovery(enabled, session, previous.current);
  const model = selected ?? (retainPending ? previous.current : null);
  previous.current = model;
  const presentedModel = model ? { ...model, loading: messagesLoading } : null;
  return (
    <Context.Provider
      value={
        enabled && session
          ? {
              sessionId: session.id,
              model: presentedModel,
              pending:
                pending ??
                (session.state === "STARTING" && model ? (lastPending.current ?? "resume") : null),
            }
          : null
      }
    >
      {children}
    </Context.Provider>
  );
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
