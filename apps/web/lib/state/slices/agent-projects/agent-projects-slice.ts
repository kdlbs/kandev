import type { StateCreator } from "zustand";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import type { AgentProjectsSlice, AgentProjectsSliceState } from "./types";

function emptyCollection() {
  return {
    byWorkspaceId: {} as Record<string, AgentProject[]>,
    loadedByWorkspaceId: {} as Record<string, boolean>,
    loadingByWorkspaceId: {} as Record<string, boolean>,
    errorByWorkspaceId: {} as Record<string, string | null>,
  };
}

export const defaultAgentProjectsState: AgentProjectsSliceState = {
  agentProjects: {
    active: emptyCollection(),
    archived: emptyCollection(),
  },
};

type ImmerSet<State> = Parameters<StateCreator<State, [["zustand/immer", never]], [], State>>[0];

function collectionFor(state: AgentProjectsSliceState, archived: boolean) {
  return archived ? state.agentProjects.archived : state.agentProjects.active;
}

export function createAgentProjectsSlice<State extends AgentProjectsSlice>(
  set: ImmerSet<State>,
): AgentProjectsSlice {
  return {
    ...defaultAgentProjectsState,
    setAgentProjects: (workspaceId, projects, archived = false) =>
      set((draft) => {
        const collection = collectionFor(draft, archived);
        collection.byWorkspaceId[workspaceId] = projects;
        collection.loadedByWorkspaceId[workspaceId] = true;
        collection.loadingByWorkspaceId[workspaceId] = false;
        collection.errorByWorkspaceId[workspaceId] = null;
      }),
    setAgentProjectsLoading: (workspaceId, loading, archived = false) =>
      set((draft) => {
        collectionFor(draft, archived).loadingByWorkspaceId[workspaceId] = loading;
      }),
    setAgentProjectsError: (workspaceId, error, archived = false) =>
      set((draft) => {
        const collection = collectionFor(draft, archived);
        collection.errorByWorkspaceId[workspaceId] = error;
        collection.loadingByWorkspaceId[workspaceId] = false;
      }),
    upsertAgentProject: (workspaceId, project, archived = false) =>
      set((draft) => {
        const collection = collectionFor(draft, archived);
        const current = collection.byWorkspaceId[workspaceId] ?? [];
        const index = current.findIndex((item) => item.id === project.id);
        if (index < 0) current.push(project);
        else current[index] = project;
        collection.byWorkspaceId[workspaceId] = current;
        collection.loadedByWorkspaceId[workspaceId] = true;
      }),
    removeAgentProject: (workspaceId, projectId, archived = false) =>
      set((draft) => {
        const collection = collectionFor(draft, archived);
        collection.byWorkspaceId[workspaceId] = (
          collection.byWorkspaceId[workspaceId] ?? []
        ).filter((project) => project.id !== projectId);
      }),
  };
}
