import { useCallback } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";

/** Old editor callbacks cannot act on a replacement workspace with the same view ID. */
export function useSidebarWorkspaceGuard() {
  const store = useAppStoreApi();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  return useCallback(
    <Args extends unknown[], Result>(action: (...args: Args) => Result) =>
      (...args: Args): Result | undefined => {
        if (!workspaceId || store.getState().workspaces.activeId !== workspaceId) return;
        return action(...args);
      },
    [store, workspaceId],
  );
}
