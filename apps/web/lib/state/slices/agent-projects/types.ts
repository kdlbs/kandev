import type { AgentProject } from "@/lib/types/http-agent-projects";

export type AgentProjectCollection = {
  byWorkspaceId: Record<string, AgentProject[]>;
  loadedByWorkspaceId: Record<string, boolean>;
  loadingByWorkspaceId: Record<string, boolean>;
  errorByWorkspaceId: Record<string, string | null>;
};

export type AgentProjectsSliceState = {
  agentProjects: {
    active: AgentProjectCollection;
    archived: AgentProjectCollection;
  };
};

export type AgentProjectsSliceActions = {
  setAgentProjects: (workspaceId: string, projects: AgentProject[], archived?: boolean) => void;
  setAgentProjectsLoading: (workspaceId: string, loading: boolean, archived?: boolean) => void;
  setAgentProjectsError: (workspaceId: string, error: string | null, archived?: boolean) => void;
  upsertAgentProject: (workspaceId: string, project: AgentProject, archived?: boolean) => void;
  removeAgentProject: (workspaceId: string, projectId: string, archived?: boolean) => void;
};

export type AgentProjectsSlice = AgentProjectsSliceState & AgentProjectsSliceActions;
