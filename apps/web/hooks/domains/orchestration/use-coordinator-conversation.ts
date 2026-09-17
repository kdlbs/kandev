import { useAppStore } from "@/components/state-provider";
import { useCallback } from "react";
import { openOrchestratorConversation } from "@/lib/api/domains/orchestration-api";
import { useOrchestrationData } from "./use-orchestration-data";

export function useCoordinatorConversation(workspaceId: string, orchestratorId: string) {
  const owner = useAppStore((s) => s.auth.user?.id);
  const load = useCallback(
    () => openOrchestratorConversation(workspaceId, orchestratorId),
    [workspaceId, orchestratorId],
  );
  return useOrchestrationData(load, owner);
}
