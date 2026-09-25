import type { AgentProjectsSliceState } from "./types";

type AgentProjectsState = AgentProjectsSliceState["agentProjects"];

export function mergeAgentProjectsState(
  incoming: AgentProjectsState | undefined,
  defaults: AgentProjectsState,
): AgentProjectsState {
  const active = incoming?.active;
  const archived = incoming?.archived;
  return {
    ...defaults,
    ...incoming,
    active: {
      ...defaults.active,
      ...active,
      byWorkspaceId: { ...defaults.active.byWorkspaceId, ...active?.byWorkspaceId },
      loadedByWorkspaceId: {
        ...defaults.active.loadedByWorkspaceId,
        ...active?.loadedByWorkspaceId,
      },
      loadingByWorkspaceId: {
        ...defaults.active.loadingByWorkspaceId,
        ...active?.loadingByWorkspaceId,
      },
      errorByWorkspaceId: {
        ...defaults.active.errorByWorkspaceId,
        ...active?.errorByWorkspaceId,
      },
    },
    archived: {
      ...defaults.archived,
      ...archived,
      byWorkspaceId: { ...defaults.archived.byWorkspaceId, ...archived?.byWorkspaceId },
      loadedByWorkspaceId: {
        ...defaults.archived.loadedByWorkspaceId,
        ...archived?.loadedByWorkspaceId,
      },
      loadingByWorkspaceId: {
        ...defaults.archived.loadingByWorkspaceId,
        ...archived?.loadingByWorkspaceId,
      },
      errorByWorkspaceId: {
        ...defaults.archived.errorByWorkspaceId,
        ...archived?.errorByWorkspaceId,
      },
    },
  };
}
