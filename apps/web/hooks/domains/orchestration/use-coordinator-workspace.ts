import { useCallback, useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchJson } from "@/lib/api/client";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listAllExecutorProfiles } from "@/lib/api/domains/settings-api";
import { listOrchestrators, listOrchestrationProfiles } from "@/lib/api/domains/orchestration-api";
import type { Workspace, ListWorkflowStepsResponse } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { useOrchestrationData } from "./use-orchestration-data";

export function useCoordinatorWorkspace(workspaceId: string) {
  const owner = useAppStore((s) => s.auth.user?.id);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const load = useCallback(async () => {
    const [workspace, workflows, steps, repositories, assignments, profiles, executors] =
      await Promise.all([
        fetchJson<Workspace>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}`),
        listWorkflows(workspaceId),
        fetchJson<ListWorkflowStepsResponse>(
          `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/workflow-steps`,
        ),
        listRepositories(workspaceId),
        listOrchestrators(workspaceId),
        listOrchestrationProfiles(workspaceId),
        listAllExecutorProfiles(),
      ]);
    return {
      workspace,
      workflows: workflows.workflows,
      steps: steps.steps,
      repositories: repositories.repositories,
      assignments: assignments.orchestrators,
      profiles: profiles.profiles,
      executors: executors.profiles,
    };
  }, [workspaceId]);
  const result = useOrchestrationData(load, owner);
  useEffect(() => {
    if (result.data?.workspace.id === workspaceId) setActiveWorkspace(workspaceId);
  }, [result.data, workspaceId, setActiveWorkspace]);
  useForegroundRefresh(result.refresh, true, workspaceId);
  return result;
}
export type CoordinatorWorkspace = NonNullable<ReturnType<typeof useCoordinatorWorkspace>["data"]>;
