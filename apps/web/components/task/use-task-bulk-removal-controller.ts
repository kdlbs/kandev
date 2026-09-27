import { useCallback, useRef } from "react";
import type { TaskSession } from "@/lib/types/http";
import { suppressTaskSessionAutoProvisioning } from "@/lib/session/session-auto-provisioning-fence";
import { useBulkSessionRemoval, type BulkSessionRemovalScope } from "./session-bulk-removal";

export function useTaskBulkRemovalController({
  taskId,
  sessionId,
  sessions,
  isLoading,
  loadSessions,
  removeById,
}: {
  taskId: string | null;
  sessionId: string | undefined;
  sessions: TaskSession[];
  isLoading: boolean;
  loadSessions: (force?: boolean) => Promise<void>;
  removeById: (id: string) => Promise<boolean>;
}) {
  const latestSessions = useRef(sessions);
  const latestLoading = useRef(isLoading);
  latestSessions.current = sessions;
  latestLoading.current = isLoading;
  const getLatestSnapshot = useCallback(async () => {
    if (taskId) await loadSessions(true);
    return { sessions: latestSessions.current, isLoading: latestLoading.current };
  }, [loadSessions, taskId]);
  const bulkRemoval = useBulkSessionRemoval({
    sessions,
    isLoading,
    remove: removeById,
    getLatestSnapshot,
    onRemoveAllConfirmed: () => taskId && suppressTaskSessionAutoProvisioning(taskId),
  });
  const request = useCallback(
    (scope: BulkSessionRemovalScope) => {
      if (sessionId) bulkRemoval.request(scope, sessionId);
    },
    [bulkRemoval, sessionId],
  );
  return { bulkRemoval, request };
}
