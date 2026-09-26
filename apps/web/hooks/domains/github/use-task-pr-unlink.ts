import { useCallback, useMemo, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getTaskPRSyncResource } from "./task-pr-sync-resource";
import { getTaskPRsForCurrentWorkspace } from "./use-task-pr-tooltip-hydration";
import { taskPRUnlinkErrorDescription, unlinkTaskPRAssociation } from "./task-pr-mutations";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";
import {
  getTaskPRUnlinkPendingIds,
  getTaskPRUnlinkPendingRevision,
  subscribeTaskPRUnlinkPending,
} from "./task-pr-unlink-registry";

export function useTaskPRUnlink(taskId: string | null) {
  const store = useAppStoreApi();
  const removeTaskPR = useAppStore((state) => state.removeTaskPR);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const resource = getTaskPRSyncResource(store);
  const { toast } = useToast();
  const { t } = useTranslation();
  const pendingRevision = useSyncExternalStore(
    useCallback((listener) => subscribeTaskPRUnlinkPending(store, listener), [store]),
    useCallback(() => getTaskPRUnlinkPendingRevision(store), [store]),
    useCallback(() => getTaskPRUnlinkPendingRevision(store), [store]),
  );
  const pendingIds = useMemo(
    () =>
      taskId
        ? getTaskPRUnlinkPendingIds(store, {
            workspaceId,
            workspaceContextGeneration,
            taskId,
          })
        : new Set<string>(),
    [pendingRevision, store, taskId, workspaceContextGeneration, workspaceId],
  );

  const unlink = useCallback(
    async (associationId: string): Promise<boolean> => {
      if (!taskId || !workspaceId || pendingIds.has(associationId)) return false;

      const state = store.getState();
      if (
        state.workspaces.activeId !== workspaceId ||
        state.workspaceContextGeneration !== workspaceContextGeneration
      ) {
        return false;
      }
      const taskPRs = getTaskPRsForCurrentWorkspace(state, taskId);
      if (!taskPRs?.some((pr) => pr.id === associationId)) return false;

      try {
        return await unlinkTaskPRAssociation({
          associationId,
          store,
          taskId,
          workspaceId,
          workspaceContextGeneration,
          isWorkspaceContextCurrent: () =>
            isCurrentWorkspaceContext(store.getState(), workspaceId, workspaceContextGeneration),
          removeTaskPR,
          invalidateSync: () =>
            resource.invalidate({ taskId, workspaceId, workspaceContextGeneration }),
        });
      } catch (error) {
        toast({
          title: t("github:failedToUnlinkPullRequest"),
          description: taskPRUnlinkErrorDescription(
            error,
            t("github:selectWorkspace"),
            t("github:thePullRequestIsStillLinked"),
          ),
          variant: "error",
        });
        return false;
      }
    },
    [
      pendingIds,
      removeTaskPR,
      resource,
      store,
      taskId,
      t,
      toast,
      workspaceContextGeneration,
      workspaceId,
    ],
  );

  return { unlink, pendingIds, canUnlink: Boolean(taskId && workspaceId) };
}
