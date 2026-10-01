import type { AppState } from "@/lib/state/store";
import type { AgentProject } from "@/lib/types/http-agent-projects";

const EMPTY_PROJECTS: AgentProject[] = [];

export function selectAgentProjects(
  state: AppState,
  workspaceId: string | null | undefined,
  archived = false,
): AgentProject[] {
  if (!workspaceId) return EMPTY_PROJECTS;
  const collection = archived ? state.agentProjects.archived : state.agentProjects.active;
  return collection.byWorkspaceId[workspaceId] ?? EMPTY_PROJECTS;
}

export function selectAgentProjectsLoaded(
  state: AppState,
  workspaceId: string | null | undefined,
  archived = false,
): boolean {
  if (!workspaceId) return false;
  const collection = archived ? state.agentProjects.archived : state.agentProjects.active;
  return collection.loadedByWorkspaceId[workspaceId] ?? false;
}

export function selectAgentProjectsLoading(
  state: AppState,
  workspaceId: string | null | undefined,
  archived = false,
): boolean {
  if (!workspaceId) return false;
  const collection = archived ? state.agentProjects.archived : state.agentProjects.active;
  return collection.loadingByWorkspaceId[workspaceId] ?? false;
}

export function selectAgentProjectsError(
  state: AppState,
  workspaceId: string | null | undefined,
  archived = false,
): string | null {
  if (!workspaceId) return null;
  const collection = archived ? state.agentProjects.archived : state.agentProjects.active;
  return collection.errorByWorkspaceId[workspaceId] ?? null;
}
