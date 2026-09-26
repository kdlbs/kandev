import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getTaskPRSyncResource } from "./task-pr-sync-resource";
import { getTaskPRsForCurrentWorkspace } from "./use-task-pr-tooltip-hydration";
import { unlinkTaskPRAssociation } from "./task-pr-mutations";
import { isCurrentWorkspaceContext } from "@/lib/state/workspace-context";

export function useTaskPRUnlink(taskId: string | null) {
  const store = useAppStoreApi();
  const removeTaskPR = useAppStore((state) => state.removeTaskPR);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceContextGeneration = useAppStore((state) => state.workspaceContextGeneration);
  const resource = getTaskPRSyncResource(store);
  const { toast } = useToast();
  const { t } = useTranslation();
  const pendingRef = useRef(new Set<string>());
  const [pendingIds, setPendingIds] = useState<ReadonlySet<string>>(() => new Set());

  const unlink = useCallback(
    async (associationId: string): Promise<boolean> => {
      if (!taskId || !workspaceId || pendingRef.current.has(associationId)) return false;

      const state = store.getState();
      if (
        state.workspaces.activeId !== workspaceId ||
        state.workspaceContextGeneration !== workspaceContextGeneration
      ) {
        return false;
      }
      const taskPRs = getTaskPRsForCurrentWorkspace(state, taskId);
      if (!taskPRs?.some((pr) => pr.id === associationId)) return false;

      pendingRef.current.add(associationId);
      setPendingIds(new Set(pendingRef.current));
      try {
        await unlinkTaskPRAssociation({
          associationId,
          taskId,
          workspaceId,
          workspaceContextGeneration,
          isWorkspaceContextCurrent: () =>
            isCurrentWorkspaceContext(store.getState(), workspaceId, workspaceContextGeneration),
          removeTaskPR,
          invalidateSync: () =>
            resource.invalidate({ taskId, workspaceId, workspaceContextGeneration }),
        });
        return true;
      } catch (error) {
        toast({
          title: t("github:failedToUnlinkPullRequest"),
          description:
            error instanceof Error ? error.message : t("github:thePullRequestIsStillLinked"),
          variant: "error",
        });
        return false;
      } finally {
        pendingRef.current.delete(associationId);
        setPendingIds(new Set(pendingRef.current));
      }
    },
    [removeTaskPR, resource, store, taskId, t, toast, workspaceContextGeneration, workspaceId],
  );

  return { unlink, pendingIds, canUnlink: Boolean(taskId && workspaceId) };
}
