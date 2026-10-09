import type { useSessionRecoveryActions } from "./use-session-recovery-actions";
import type { SessionRecoveryNoticeKind } from "./use-session-resumption";
import type { SessionDeliveryRecoveryResponse } from "@/lib/services/session-recovery-service";
import { useCallback, useRef } from "react";
import type { SessionRecoveryAction } from "@/lib/services/session-recovery-service";
import type { WorkspaceRecoveryProjection } from "@/lib/types/http";

export type RecoveryOperation = { requestKey: string; sessionKey: string; operationId: number };

/** Fences in-flight recovery calls so a stale response cannot write newer state. */
export function useRecoveryOperationFence(
  requestKey: string,
  sessionKey: string,
  errorStamp?: string | null,
) {
  const activeRequestKeyRef = useRef(requestKey);
  const activeSessionKeyRef = useRef(sessionKey);
  const latestErrorStampRef = useRef(errorStamp);
  const operationGenerationRef = useRef(0);
  activeSessionKeyRef.current = sessionKey;
  latestErrorStampRef.current = errorStamp;
  if (activeRequestKeyRef.current !== requestKey) {
    activeRequestKeyRef.current = requestKey;
    operationGenerationRef.current += 1;
  }

  const beginOperation = useCallback(
    (): RecoveryOperation => ({
      requestKey,
      sessionKey,
      operationId: ++operationGenerationRef.current,
    }),
    [requestKey, sessionKey],
  );

  const isCurrentOperation = useCallback(
    (operation: RecoveryOperation) =>
      activeRequestKeyRef.current === operation.requestKey &&
      activeSessionKeyRef.current === operation.sessionKey &&
      operationGenerationRef.current === operation.operationId,
    [],
  );

  const isCurrentSession = useCallback(
    (operation: RecoveryOperation) => activeSessionKeyRef.current === operation.sessionKey,
    [],
  );

  const matchesLatestErrorStamp = useCallback(
    (stamp: string | undefined) => Boolean(stamp && latestErrorStampRef.current === stamp),
    [],
  );

  return { beginOperation, isCurrentOperation, isCurrentSession, matchesLatestErrorStamp };
}

function compareDecimalIdentity(left: string, right: string): number | null {
  if (!/^\d+$/.test(left) || !/^\d+$/.test(right)) return left === right ? 0 : null;
  const normalizedLeft = left.replace(/^0+(?=\d)/, "");
  const normalizedRight = right.replace(/^0+(?=\d)/, "");
  if (normalizedLeft.length !== normalizedRight.length)
    return normalizedLeft.length < normalizedRight.length ? -1 : 1;
  if (normalizedLeft === normalizedRight) return 0;
  return normalizedLeft < normalizedRight ? -1 : 1;
}

export function isOlderWorkspaceRecoveryProjection(
  incoming: WorkspaceRecoveryProjection,
  current: WorkspaceRecoveryProjection | null,
): boolean {
  if (!current || incoming.environment_id !== current.environment_id) return false;
  const generationOrder = compareDecimalIdentity(
    incoming.ownership_generation,
    current.ownership_generation,
  );
  if (generationOrder !== null && generationOrder !== 0) return generationOrder < 0;
  if (incoming.ownership_generation !== current.ownership_generation) return true;
  const revisionOrder = compareDecimalIdentity(incoming.revision, current.revision);
  const sameAttempt =
    incoming.operation_id === current.operation_id && incoming.attempt_id === current.attempt_id;
  if (!sameAttempt) return revisionOrder === null || revisionOrder <= 0;
  return revisionOrder !== null && revisionOrder < 0;
}

export function workspaceRecoveryMatchesFailure(
  recovery: WorkspaceRecoveryProjection | null | undefined,
  sessionId: string,
  errorStamp: string | null | undefined,
): boolean {
  return Boolean(
    recovery?.session_id === sessionId &&
    errorStamp &&
    recovery.error_stamp === errorStamp &&
    !recovery.workspace_complete &&
    !recovery.agent_ready,
  );
}

export function currentRecoveryValue<T>(isCurrent: boolean, value: T): T | null {
  return isCurrent ? value : null;
}

export type SessionRecoveryBusyAction = SessionRecoveryAction | "restore" | null;
export type WorkspaceRecoveryStatusCheck = "idle" | "checking" | "unresolved";

export type ManualSessionRecoveryFailure = {
  operation: "resume" | "restore_workspace";
  sessionId: string;
  errorStamp: string | null;
  requestKey: string;
  operationId: number;
};

export type SessionRecoveryActions = Omit<
  ReturnType<typeof useSessionRecoveryActions>,
  | "workspaceRecoveryMatchesCurrentFailure"
  | "recoveryNoticeKind"
  | "deliveryRecoveryNotice"
  | "deliveryRecoveryResult"
  | "clearInspectionContentionNotice"
> & {
  workspaceRecoveryMatchesCurrentFailure?: boolean;
  recoveryNoticeKind?: SessionRecoveryNoticeKind | null;
  deliveryRecoveryNotice?: string | null;
  deliveryRecoveryResult?: SessionDeliveryRecoveryResponse | null;
  clearInspectionContentionNotice?: () => void;
};
