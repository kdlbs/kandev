import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import { useCallback, useRef, useState } from "react";

export type BulkSessionRemovalScope = "others" | "all";
export type BulkSessionRemovalReason = "loading" | "error" | "empty" | "active" | null;

export type BulkSessionRemovalSnapshot = {
  scope: BulkSessionRemovalScope;
  selectedSessionId: string;
  targetIds: string[];
  targetStates: Array<[string, TaskSessionState]>;
  eligible: boolean;
  reason: BulkSessionRemovalReason;
};

export type BulkSessionRemovalConfirmationResult =
  | { stale: true }
  | { stale: false; removed: number; remaining: number; failed: boolean }
  | null;

function isActiveDeletionTarget(state: TaskSessionState): boolean {
  return state === "RUNNING" || state === "STARTING";
}

function orderedTargetSessions(
  scope: BulkSessionRemovalScope,
  selectedSessionId: string,
  sessions: TaskSession[],
): TaskSession[] {
  const candidates = sessions.filter(
    (session) => scope === "all" || session.id !== selectedSessionId,
  );
  return [
    ...candidates.filter((session) => !session.is_primary && session.id !== selectedSessionId),
    ...candidates.filter((session) => session.is_primary && session.id !== selectedSessionId),
    ...candidates.filter((session) => session.id === selectedSessionId),
  ];
}

export function buildBulkSessionRemovalSnapshot(
  scope: BulkSessionRemovalScope,
  selectedSessionId: string,
  sessions: TaskSession[],
  isLoading: boolean,
  hasError = false,
): BulkSessionRemovalSnapshot {
  const targets = orderedTargetSessions(scope, selectedSessionId, sessions);
  const targetStates = targets.map(
    (session) => [session.id, session.state] as [string, TaskSessionState],
  );
  let reason: BulkSessionRemovalReason = null;
  if (isLoading) reason = "loading";
  else if (hasError) reason = "error";
  else if (targets.length === 0) reason = "empty";
  else if (targets.some((session) => isActiveDeletionTarget(session.state))) reason = "active";
  return {
    scope,
    selectedSessionId,
    targetIds: targets.map((session) => session.id),
    targetStates,
    eligible: reason === null,
    reason,
  };
}

function didSelectedSessionDisappear(
  snapshot: BulkSessionRemovalSnapshot,
  latest: { sessions: TaskSession[]; isLoading: boolean; hasError?: boolean },
): boolean {
  return (
    snapshot.scope === "others" &&
    !latest.isLoading &&
    !latest.hasError &&
    !latest.sessions.some((session) => session.id === snapshot.selectedSessionId)
  );
}

export function isBulkSessionRemovalSnapshotCurrent(
  snapshot: BulkSessionRemovalSnapshot,
  sessions: TaskSession[],
  isLoading: boolean,
  hasError = false,
): boolean {
  if (didSelectedSessionDisappear(snapshot, { sessions, isLoading, hasError })) return false;
  const current = buildBulkSessionRemovalSnapshot(
    snapshot.scope,
    snapshot.selectedSessionId,
    sessions,
    isLoading,
    hasError,
  );
  return (
    current.eligible &&
    current.targetIds.length === snapshot.targetIds.length &&
    current.targetIds.every((id, index) => id === snapshot.targetIds[index]) &&
    current.targetStates.every(
      ([id, state], index) =>
        id === snapshot.targetStates[index]?.[0] && state === snapshot.targetStates[index]?.[1],
    )
  );
}

export async function executeBulkSessionRemoval(
  targetIds: string[],
  remove: (sessionId: string) => Promise<boolean>,
  onProgress?: (removed: number) => void,
): Promise<{ removed: number; remaining: number; failed: boolean }> {
  let removed = 0;
  for (const sessionId of targetIds) {
    let succeeded = false;
    try {
      succeeded = await remove(sessionId);
    } catch {
      succeeded = false;
    }
    if (!succeeded) {
      return { removed, remaining: targetIds.length - removed, failed: true };
    }
    removed += 1;
    onProgress?.(removed);
  }
  return { removed, remaining: 0, failed: false };
}

function refreshInvalidSnapshot(
  snapshot: BulkSessionRemovalSnapshot,
  latest: { sessions: TaskSession[]; isLoading: boolean; hasError?: boolean },
): BulkSessionRemovalSnapshot | null {
  if (
    isBulkSessionRemovalSnapshotCurrent(
      snapshot,
      latest.sessions,
      latest.isLoading,
      latest.hasError,
    )
  ) {
    return null;
  }
  return buildBulkSessionRemovalSnapshot(
    snapshot.scope,
    snapshot.selectedSessionId,
    latest.sessions,
    latest.isLoading,
    latest.hasError,
  );
}

export function useBulkSessionRemoval({
  sessions,
  isLoading,
  hasError = false,
  remove,
  getLatestSnapshot,
  onInvalidSnapshot,
  onRemoveAllConfirmed,
  onComplete,
}: {
  sessions: TaskSession[];
  isLoading: boolean;
  hasError?: boolean;
  remove: (sessionId: string) => Promise<boolean>;
  getLatestSnapshot?: () => Promise<{
    sessions: TaskSession[];
    isLoading: boolean;
    hasError?: boolean;
  }>;
  onInvalidSnapshot?: (reason: BulkSessionRemovalReason) => void;
  onRemoveAllConfirmed?: () => void;
  onComplete?: (result: { removed: number; remaining: number; failed: boolean }) => void;
}) {
  const [snapshot, setSnapshot] = useState<BulkSessionRemovalSnapshot | null>(null);
  const [pending, setPending] = useState(false);
  const [removedCount, setRemovedCount] = useState(0);
  const [wasRefreshed, setWasRefreshed] = useState(false);
  const pendingRef = useRef(false);
  const request = useCallback(
    (scope: BulkSessionRemovalScope, selectedSessionId: string) => {
      const next = buildBulkSessionRemovalSnapshot(
        scope,
        selectedSessionId,
        sessions,
        isLoading,
        hasError,
      );
      if (pendingRef.current) return next;
      if (next.eligible) setSnapshot(next);
      setRemovedCount(0);
      setWasRefreshed(false);
      return next;
    },
    [hasError, isLoading, sessions],
  );
  const cancel = useCallback(() => {
    if (pendingRef.current) return;
    setSnapshot(null);
    setWasRefreshed(false);
  }, []);
  const confirm = useCallback(async () => {
    if (!snapshot || pendingRef.current) return null;
    pendingRef.current = true;
    setPending(true);
    let latest: { sessions: TaskSession[]; isLoading: boolean; hasError?: boolean };
    try {
      latest = getLatestSnapshot ? await getLatestSnapshot() : { sessions, isLoading };
    } catch {
      latest = { sessions: [], isLoading: true };
    }
    const selectedSessionMissing = didSelectedSessionDisappear(snapshot, latest);
    const refreshed = refreshInvalidSnapshot(snapshot, latest);
    if (refreshed) {
      const canReviewAgain = refreshed.eligible && !selectedSessionMissing;
      setSnapshot(canReviewAgain ? refreshed : null);
      setWasRefreshed(canReviewAgain);
      setPending(false);
      pendingRef.current = false;
      onInvalidSnapshot?.(selectedSessionMissing ? null : refreshed.reason);
      return { stale: true } satisfies BulkSessionRemovalConfirmationResult;
    }
    if (snapshot.scope === "all") onRemoveAllConfirmed?.();
    const result = await executeBulkSessionRemoval(snapshot.targetIds, remove, setRemovedCount);
    setPending(false);
    pendingRef.current = false;
    setSnapshot(null);
    setWasRefreshed(false);
    onComplete?.(result);
    return { ...result, stale: false } satisfies BulkSessionRemovalConfirmationResult;
  }, [
    getLatestSnapshot,
    isLoading,
    onComplete,
    onInvalidSnapshot,
    onRemoveAllConfirmed,
    remove,
    sessions,
    snapshot,
  ]);
  return {
    snapshot,
    pending,
    removedCount,
    wasRefreshed,
    request,
    cancel,
    confirm,
  };
}
