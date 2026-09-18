import { useEffect, useMemo, useSyncExternalStore } from "react";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import {
  getWorkspacePage,
  getWorkspaceReceiver,
  type WorkspacePages,
} from "@/lib/api/domains/assistant-workspace-api";
import { AssistantCollection } from "@/lib/orchestration/assistant-observation";
import { useOrchestrationData } from "./use-orchestration-data";
export function useWorkspaceReceiver(binding: AssistantBinding, revision: number) {
  return useOrchestrationData(
    getWorkspaceReceiver,
    `${binding.owner_user_id}:${binding.id}:${binding.version}:${revision}`,
  );
}
export function useWorkspacePage<K extends keyof WorkspacePages>(
  kind: K,
  binding: AssistantBinding,
  revision: number,
  id = "",
) {
  const key = `${binding.owner_user_id}:${binding.id}:${binding.version}`;
  const view = useMemo(
    () =>
      new AssistantCollection<WorkspacePages[K]>((after, signal) =>
        getWorkspacePage(kind, id, after, signal),
      ),
    [key, kind, id],
  );
  const state = useSyncExternalStore(view.subscribe, view.getSnapshot, view.getSnapshot);
  useEffect(() => {
    view.activate();
    return view.dispose;
  }, [view]);
  useEffect(() => {
    void view.refresh();
  }, [view, revision]);
  return { ...state, refresh: view.refresh, loadMore: view.loadMore };
}
