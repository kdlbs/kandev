import { useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { updateTask } from "@/lib/api";
import type { TaskSession } from "@/lib/types/http";
import {
  clearTaskSessionAutoProvisioningSuppression,
  suppressTaskSessionAutoProvisioning,
} from "@/lib/session/session-auto-provisioning-fence";
import {
  useBulkSessionRemoval,
  type BulkSessionRemovalReason,
  type BulkSessionRemovalScope,
} from "./use-bulk-session-removal";

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
  const suppressedTaskIdRef = useRef<string | null>(null);
  const hasError = useAppStore((state) =>
    taskId ? !!state.taskSessionsByTask.errorByTaskId?.[taskId] : false,
  );
  const isLoaded = useAppStore((state) =>
    taskId ? !!state.taskSessionsByTask.loadedByTaskId[taskId] : false,
  );
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
      // This forced read has settled; another observer's refresh does not invalidate it.
      isLoading: !taskId || !state.taskSessionsByTask.loadedByTaskId[taskId],
      hasError: taskId ? !!state.taskSessionsByTask.errorByTaskId?.[taskId] : false,
    };
  }, [loadSessions, store, taskId]);
  const bulkRemoval = useBulkSessionRemoval({
    sessions,
    isLoading: isLoading || !isLoaded,
    hasError,
    remove: removeById,
    getLatestSnapshot,
    onRemoveAllConfirmed: async () => {
      if (!taskId) return;
      await updateTask(taskId, { metadata: { bulk_session_removal_suppress_ensure: true } });
      suppressedTaskIdRef.current = taskId;
      try {
        suppressTaskSessionAutoProvisioning(taskId);
      } catch {
        // The durable task marker still protects other tabs when browser storage fails.
      }
    },
    onInvalidSnapshot: (reason) =>
      reason ? reportIneligible(reason) : toast({ title: t("task:sessionsChangedReviewAgain") }),
    onComplete: async ({ removed, remaining, failed }) => {
      if (failed) {
        toast({
          title: t("task:bulkRemovalPartial", { removed, remaining }),
          variant: "error",
        });
        const suppressedTaskId = suppressedTaskIdRef.current;
        if (suppressedTaskId) {
          try {
            await updateTask(suppressedTaskId, {
              metadata: { bulk_session_removal_suppress_ensure: false },
            });
            clearTaskSessionAutoProvisioningSuppression(suppressedTaskId);
          } catch {
            toast({ title: t("task:bulkRemovalUnavailable_error"), variant: "error" });
          }
        }
      }
      suppressedTaskIdRef.current = null;
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
