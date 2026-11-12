"use client";

import { createContext, useContext, useEffect, useRef, type ReactNode } from "react";
import type { Message, TaskSession } from "@/lib/types/http";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";
import { useOptionalAppStore } from "@/components/state-provider";
import {
  selectActiveSessionRecovery,
  type ActiveSessionRecovery,
} from "@/lib/active-session-recovery";
import { useTaskLaunchErrorContext } from "../task-launch-error-context";
import { usePendingSessionRecovery } from "@/hooks/domains/session/session-recovery-pending";
import { useSessionRecoveryGuidance } from "@/hooks/domains/session/use-session-recovery-guidance";
import { matchingAutomaticRecovery } from "@/lib/session-recovery-presentation";
import { readAgentDeliveryRecovery } from "@/lib/session-agent-delivery-recovery";
import { invalidateSessionRecoveryGuidance } from "@/lib/session/session-recovery-guidance";
import { lastAgentErrorStamp, readLastAgentError } from "@/lib/session-last-agent-error";

type RecoveryContext = {
  sessionId: string;
  onDeliveryReconciled?: () => void;
  model: ActiveSessionRecovery | null;
  pending: import("@/hooks/domains/session/use-session-recovery-actions").SessionRecoveryBusyAction;
};
const Context = createContext<RecoveryContext | null>(null);
const EMPTY_SESSION_ITEMS: Record<string, TaskSession> = {};

export function SessionRecoveryProvider({
  session,
  messages,
  taskId,
  enabled,
  messagesLoading = false,
  onDeliveryReconciled,
  children,
}: {
  session: TaskSession | null | undefined;
  messages: Message[];
  taskId: string | null;
  enabled: boolean;
  messagesLoading?: boolean;
  onDeliveryReconciled?: () => void;
  children: ReactNode;
}) {
  const recoveryKey = `${taskId ?? ""}\u0000${session?.id ?? ""}`;
  const pending = usePendingSessionRecovery(recoveryKey);
  const lastPending = useLastPendingRecovery(pending, recoveryKey);
  const previous = useRef<ActiveSessionRecovery | null>(null);
  const selected = useCurrentRecovery(session, messages, enabled);
  const { automaticRecovery, guidanceModel, guidanceSubmissionId } = useRequestLocalGuidance(
    taskId,
    session,
  );
  const retainPending = retainRecovery(enabled, session, previous.current);
  const committedRestoration = committedDeliveryRestoration(session);
  const newRecoveryRequest = useIsNewRequestAfterRestoration(
    committedRestoration,
    automaticRecovery,
  );
  const model = selectRecoveryModel({
    selected,
    restoration: committedRestoration,
    isNewRequest: newRecoveryRequest,
    guidanceModel,
    guidanceSubmissionId,
    sessionId: session?.id,
    automaticRecovery,
    retainPending,
    previousModel: previous.current,
  });
  previous.current = model;
  const presentedModel = model ? { ...model, loading: messagesLoading } : null;
  return (
    <Context.Provider
      value={recoveryContextValue({
        enabled,
        session,
        model: presentedModel,
        pending,
        lastPending,
        onDeliveryReconciled,
      })}
    >
      {children}
    </Context.Provider>
  );
}

function committedDeliveryRestoration(session: TaskSession | null | undefined) {
  if (!session) return null;
  const recovery = readAgentDeliveryRecovery(session.metadata);
  return recovery?.phase === "restored" &&
    Array.isArray(session.session_recovery_blocks) &&
    session.session_recovery_blocks.length === 0
    ? recovery
    : null;
}

function selectRecoveryModel({
  selected,
  restoration,
  isNewRequest,
  guidanceModel,
  guidanceSubmissionId,
  sessionId,
  automaticRecovery,
  retainPending,
  previousModel,
}: {
  selected: ActiveSessionRecovery | null;
  restoration: ReturnType<typeof committedDeliveryRestoration>;
  isNewRequest: boolean;
  guidanceModel: ActiveSessionRecovery | null;
  guidanceSubmissionId: string | undefined;
  sessionId: string | undefined;
  automaticRecovery: ReturnType<typeof matchingAutomaticRecovery>;
  retainPending: boolean;
  previousModel: ActiveSessionRecovery | null;
}): ActiveSessionRecovery | null {
  const selectedModel = recoveryModelBelongsToSubmission(selected, restoration?.submissionId)
    ? null
    : selected;
  const currentGuidanceModel =
    restoration && guidanceSubmissionId === restoration.submissionId ? null : guidanceModel;
  const requestLocalModel =
    restoration && !isNewRequest ? null : requestLocalRecoveryModel(sessionId, automaticRecovery);
  const historicalModel = recoveryModelBelongsToSubmission(previousModel, restoration?.submissionId)
    ? null
    : previousModel;
  const retainedPreviousModel =
    restoration && !isNewRequest && historicalModel?.requestLocal ? null : historicalModel;

  return (
    selectedModel ??
    currentGuidanceModel ??
    chooseRecoveryModel(null, retainPending, retainedPreviousModel, requestLocalModel)
  );
}

function recoveryModelBelongsToSubmission(
  model: ActiveSessionRecovery | null,
  submissionId: string | undefined,
): boolean {
  if (!model || !submissionId) return false;
  const metadataSubmissionId =
    model.metadata && "delivery_submission_id" in model.metadata
      ? model.metadata.delivery_submission_id
      : undefined;
  return (
    model.error?.deliverySubmissionId === submissionId || metadataSubmissionId === submissionId
  );
}

function useIsNewRequestAfterRestoration(
  restoration: ReturnType<typeof committedDeliveryRestoration>,
  automaticRecovery: ReturnType<typeof matchingAutomaticRecovery>,
): boolean {
  const requestIdentity = automaticRecovery?.requestIdentity;
  const requestKey = requestIdentity
    ? `${requestIdentity.taskId}\u0000${requestIdentity.sessionId}\u0000${requestIdentity.generation}\u0000${requestIdentity.attemptId}`
    : null;
  const restorationKey = restoration
    ? `${restoration.incarnationId}\u0000${restoration.harnessGeneration}\u0000${restoration.submissionId}`
    : null;
  const initialRequest = useRef<{ restorationKey: string | null; requestKey: string | null }>({
    restorationKey: null,
    requestKey: null,
  });
  if (restorationKey === null) {
    initialRequest.current = { restorationKey: null, requestKey: null };
  } else if (initialRequest.current.restorationKey !== restorationKey) {
    initialRequest.current = { restorationKey, requestKey };
  }
  return Boolean(
    restorationKey &&
    requestKey &&
    initialRequest.current.restorationKey === restorationKey &&
    initialRequest.current.requestKey !== requestKey,
  );
}

function useRequestLocalGuidance(taskId: string | null, session: TaskSession | null | undefined) {
  const sessionItems = useOptionalAppStore(
    (state) => state.taskSessions.items,
    EMPTY_SESSION_ITEMS,
  );
  const launchErrorContext = useTaskLaunchErrorContext();
  const automaticRecovery = matchingAutomaticRecovery(
    launchErrorContext?.automaticRecovery,
    taskId,
    session?.id,
  );
  const errorStamp = sessionRecoveryErrorStamp(
    session,
    launchErrorContext?.statusSummary?.active_error,
  );
  const { owner, entry } = useSessionRecoveryGuidance(
    taskId,
    session?.id ?? null,
    session,
    errorStamp,
  );
  useDeletedGuidanceCleanup(owner, taskId, session, sessionItems);
  return {
    automaticRecovery,
    guidanceModel: session && entry ? continuationGuidanceModel(session.id) : null,
    guidanceSubmissionId: entry?.identity.deliverySubmissionId,
  };
}

function useLastPendingRecovery(
  pending: RecoveryContext["pending"],
  key: string,
): RecoveryContext["pending"] {
  const lastPending = useRef({ key, pending });
  if (lastPending.current.key !== key) {
    lastPending.current = { key, pending };
  } else if (pending) {
    lastPending.current.pending = pending;
  }
  return lastPending.current.pending;
}

function useDeletedGuidanceCleanup(
  owner: ReturnType<typeof useSessionRecoveryGuidance>["owner"],
  taskId: string | null,
  session: TaskSession | null | undefined,
  sessionItems: Record<string, TaskSession>,
) {
  const previousTarget = useRef<{ taskId: string; sessionId: string } | null>(null);
  useEffect(() => {
    const previous = previousTarget.current;
    let next = previous;
    if (previous && !sessionItems[previous.sessionId]) {
      invalidateSessionRecoveryGuidance(owner, previous.taskId, previous.sessionId);
      next = null;
    }
    if (session && taskId) next = { taskId, sessionId: session.id };
    previousTarget.current = next;
  }, [owner, session, sessionItems, taskId]);
}

function sessionRecoveryErrorStamp(
  session: TaskSession | null | undefined,
  activeError?: TaskStatusSummaryActiveError | null,
) {
  if (!session) return undefined;
  if (activeError?.scope === "session" && activeError.session_id === session.id) {
    return activeError.stamp;
  }
  const persistedError = readLastAgentError(session.metadata);
  return persistedError ? lastAgentErrorStamp(persistedError) : undefined;
}

function continuationGuidanceModel(sessionId: string): ActiveSessionRecovery {
  return { sessionId, kind: "session_restore_required", requestLocal: true };
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
  onDeliveryReconciled,
}: {
  enabled: boolean;
  session: TaskSession | null | undefined;
  model: ActiveSessionRecovery | null;
  pending: RecoveryContext["pending"];
  lastPending: RecoveryContext["pending"];
  onDeliveryReconciled?: () => void;
}): RecoveryContext | null {
  if (!enabled || !session) return null;
  const startingPending = session.state === "STARTING" && model ? (lastPending ?? "resume") : null;
  return {
    sessionId: session.id,
    onDeliveryReconciled,
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
    return {
      sessionId,
      kind: "recovery_inspection_busy",
      summary: recovery.notice ?? undefined,
      requestLocal: true,
    };
  }
  if (recovery?.error)
    return { sessionId, kind: "generic", summary: recovery.error, requestLocal: true };
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
