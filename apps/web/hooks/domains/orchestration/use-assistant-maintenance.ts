import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import {
  getImprovement,
  getMaintenanceArtifact,
  getMaintenanceFile,
  getMaintenancePage,
  type MaintenancePages,
} from "@/lib/api/domains/assistant-maintenance-api";
import { AssistantCollection } from "@/lib/orchestration/assistant-observation";
import { useOrchestrationData } from "./use-orchestration-data";
export function useImprovement(binding: AssistantBinding, id: string, revision: number) {
  const load = useCallback(() => getImprovement(id), [id]);
  const state = useOrchestrationData(
    load,
    `${binding.owner_user_id}:${binding.id}:${binding.version}:${id}`,
  );
  useEffect(() => {
    state.refresh();
  }, [revision, state.refresh]);
  return state;
}
export function useMaintenanceArtifact(binding: AssistantBinding, id: string, revision: number) {
  const load = useCallback(() => getMaintenanceArtifact(id), [id]);
  return useOrchestrationData(
    load,
    `${binding.owner_user_id}:${binding.id}:${binding.version}:${id}:${revision}`,
  );
}
export function useMaintenanceFile(
  binding: AssistantBinding,
  id: string,
  grant: number,
  path: string,
) {
  const load = useCallback(
    () =>
      path ? getMaintenanceFile(id, path, binding.version, grant) : Promise.resolve(undefined),
    [id, path, binding.version, grant],
  );
  return useOrchestrationData(
    load,
    `${binding.owner_user_id}:${binding.id}:${binding.version}:${id}`,
  );
}
export function useMaintenancePage<K extends keyof MaintenancePages>(
  kind: K,
  binding: AssistantBinding,
  id: string,
  revision: number,
) {
  const key = `${binding.owner_user_id}:${binding.id}:${binding.version}:${id}`;
  const view = useMemo(
    () =>
      new AssistantCollection<MaintenancePages[K]>((after, signal) =>
        getMaintenancePage(kind, id, after, signal),
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
