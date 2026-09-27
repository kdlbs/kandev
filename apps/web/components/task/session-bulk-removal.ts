import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import { useCallback, useState } from "react";

export type BulkSessionRemovalScope = "others" | "all";
export type BulkSessionRemovalReason = "loading" | "empty" | "active" | null;

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
    ...candidates.filter((session) => !session.is_primary),
    ...candidates.filter((session) => session.is_primary),
  ];
}

export function buildBulkSessionRemovalSnapshot(
  scope: BulkSessionRemovalScope,
  selectedSessionId: string,
  sessions: TaskSession[],
  isLoading: boolean,
): BulkSessionRemovalSnapshot {
  const targets = orderedTargetSessions(scope, selectedSessionId, sessions);
  const targetStates = targets.map(
    (session) => [session.id, session.state] as [string, TaskSessionState],
  );
  let reason: BulkSessionRemovalReason = null;
  if (isLoading) reason = "loading";
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

export function isBulkSessionRemovalSnapshotCurrent(
  snapshot: BulkSessionRemovalSnapshot,
  sessions: TaskSession[],
  isLoading: boolean,
): boolean {
  const current = buildBulkSessionRemovalSnapshot(
    snapshot.scope,
    snapshot.selectedSessionId,
    sessions,
    isLoading,
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
): Promise<{ removed: number; remaining: number; failed: boolean }> {
  let removed = 0;
  for (const sessionId of targetIds) {
    if (!(await remove(sessionId))) {
      return { removed, remaining: targetIds.length - removed, failed: true };
    }
    removed += 1;
  }
  return { removed, remaining: 0, failed: false };
}

export function useBulkSessionRemoval({
  sessions,
  isLoading,
  remove,
  getLatestSnapshot,
  onInvalidSnapshot,
  onRemoveAllConfirmed,
  onComplete,
}: {
  sessions: TaskSession[];
  isLoading: boolean;
  remove: (sessionId: string) => Promise<boolean>;
  getLatestSnapshot?: () => Promise<{ sessions: TaskSession[]; isLoading: boolean }>;
  onInvalidSnapshot?: () => void;
  onRemoveAllConfirmed?: () => void;
  onComplete?: (result: { removed: number; remaining: number; failed: boolean }) => void;
}) {
  const [snapshot, setSnapshot] = useState<BulkSessionRemovalSnapshot | null>(null);
  const [pending, setPending] = useState(false);
  const [wasRefreshed, setWasRefreshed] = useState(false);
  const request = useCallback(
    (scope: BulkSessionRemovalScope, selectedSessionId: string) => {
      const next = buildBulkSessionRemovalSnapshot(scope, selectedSessionId, sessions, isLoading);
      if (next.eligible) setSnapshot(next);
      setWasRefreshed(false);
      return next;
    },
    [isLoading, sessions],
  );
  const cancel = useCallback(() => {
    setSnapshot(null);
    setWasRefreshed(false);
  }, []);
  const confirm = useCallback(async () => {
    if (!snapshot || pending) return null;
    setPending(true);
    const latest = getLatestSnapshot ? await getLatestSnapshot() : { sessions, isLoading };
    if (!isBulkSessionRemovalSnapshotCurrent(snapshot, latest.sessions, latest.isLoading)) {
      const refreshed = buildBulkSessionRemovalSnapshot(
        snapshot.scope,
        snapshot.selectedSessionId,
        latest.sessions,
        latest.isLoading,
      );
      setSnapshot(refreshed.eligible ? refreshed : null);
      setWasRefreshed(refreshed.eligible);
      setPending(false);
      onInvalidSnapshot?.();
      return { stale: true } satisfies BulkSessionRemovalConfirmationResult;
    }
    if (snapshot.scope === "all") onRemoveAllConfirmed?.();
    const result = await executeBulkSessionRemoval(snapshot.targetIds, remove);
    setPending(false);
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
    pending,
    remove,
    sessions,
    snapshot,
  ]);
  return { snapshot, pending, wasRefreshed, request, cancel, confirm };
}
