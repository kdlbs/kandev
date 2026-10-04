"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useAgentProjects } from "./use-agent-projects";
import {
  resolveAgentProjectSessionProfile,
  type AgentProjectSessionProfilePolicy,
} from "@/components/task/agent-project-session-profile";

type UseAgentProjectSessionProfileInput = {
  taskId: string;
  workspaceId?: string | null;
  currentSessionTaskId?: string;
  currentSessionProfileId?: string;
};

type TaskProjection = {
  id: string;
  workspaceId?: string;
  agent_project_id?: string;
  agent_project_tier?: string;
  agent_project_profile_id?: string;
  metadata?: Record<string, unknown> | null;
};

export function useAgentProjectSessionProfile({
  taskId,
  workspaceId,
  currentSessionTaskId,
  currentSessionProfileId,
}: UseAgentProjectSessionProfileInput): AgentProjectSessionProfilePolicy {
  const task = useAppStore((state) => {
    return (state.taskOverview.byId[taskId] ??
      state.kanban.tasks.find((entry) => entry.id === taskId)) as TaskProjection | undefined;
  });
  const projectsEnabled = useFeature("agentProjects");
  const resolvedWorkspaceId = workspaceId || task?.workspaceId;
  const { projects, loaded } = useAgentProjects(resolvedWorkspaceId, false, projectsEnabled);

  return useMemo(
    () =>
      resolveAgentProjectSessionProfile({
        taskId,
        task,
        projects,
        projectsEnabled,
        projectsLoaded: loaded,
        currentSessionTaskId,
        currentSessionProfileId,
      }),
    [
      taskId,
      task,
      projects,
      projectsEnabled,
      loaded,
      currentSessionTaskId,
      currentSessionProfileId,
    ],
  );
}
