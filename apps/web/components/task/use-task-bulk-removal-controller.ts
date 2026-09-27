import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import type { TaskSession } from "@/lib/types/http";
import { suppressTaskSessionAutoProvisioning } from "@/lib/session/session-auto-provisioning-fence";
import {
  useBulkSessionRemoval,
  type BulkSessionRemovalReason,
  type BulkSessionRemovalScope,
} from "./session-bulk-removal";

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
  const { t } = useTranslation();
  const { toast } = useToast();
  const store = useAppStoreApi();
  const reportIneligible = useCallback(
    (reason: BulkSessionRemovalReason) => {
      if (!reason) return;
      toast({ title: t(`task:bulkRemovalUnavailable_${reason}`), variant: "error" });
    },
    [t, toast],
  );
  const getLatestSnapshot = useCallback(async () => {
    if (taskId) await loadSessions(true);
    const state = store.getState();
    return {
      sessions: taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? []) : [],
      isLoading: !taskId || !!state.taskSessionsByTask.errorByTaskId?.[taskId],
    };
  }, [loadSessions, store, taskId]);
  const bulkRemoval = useBulkSessionRemoval({
    sessions,
    isLoading,
    remove: removeById,
    getLatestSnapshot,
    onRemoveAllConfirmed: () => taskId && suppressTaskSessionAutoProvisioning(taskId),
    onInvalidSnapshot: () => toast({ title: t("task:sessionsChangedReviewAgain") }),
    onComplete: ({ removed, remaining, failed }) => {
      if (failed) {
        toast({
          title: t("task:bulkRemovalPartial", { removed, remaining }),
          variant: "error",
        });
      }
    },
  });
  const request = useCallback(
    (scope: BulkSessionRemovalScope, targetSessionId = sessionId) => {
      if (!targetSessionId) return;
      reportIneligible(bulkRemoval.request(scope, targetSessionId).reason);
    },
    [bulkRemoval, reportIneligible, sessionId],
  );
  return { bulkRemoval, request };
}
