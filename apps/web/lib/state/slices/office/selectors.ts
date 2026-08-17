import type { AppState } from "@/lib/state/store";
import type { AgentProfile, DashboardData, InboxItem, Project } from "./types";

/**
 * Reads for the workspace-keyed office collections.
 *
 * Every one of them resolves against `workspaces.activeId`, so a component
 * asks for "the office data" and gets the active workspace's copy — never
 * another workspace's, which is what a flat field could not guarantee once
 * this data started loading outside the office route.
 *
 * The empty fallbacks are module constants, not fresh literals. A selector
 * returning `?? []` mints a new array on every call, and zustand compares
 * selector output by identity: a component subscribed to a workspace with no
 * entry yet would re-render on every unrelated store write.
 */
const EMPTY_AGENT_PROFILES: AgentProfile[] = [];
const EMPTY_PROJECTS: Project[] = [];
const EMPTY_INBOX_ITEMS: InboxItem[] = [];

export function selectOfficeAgentProfiles(state: AppState): AgentProfile[] {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return EMPTY_AGENT_PROFILES;
  return state.office.agentProfilesByWorkspaceId[workspaceId] ?? EMPTY_AGENT_PROFILES;
}

export function selectOfficeProjects(state: AppState): Project[] {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return EMPTY_PROJECTS;
  return state.office.projectsByWorkspaceId[workspaceId] ?? EMPTY_PROJECTS;
}

export function selectOfficeInboxItems(state: AppState): InboxItem[] {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return EMPTY_INBOX_ITEMS;
  return state.office.inboxItemsByWorkspaceId[workspaceId] ?? EMPTY_INBOX_ITEMS;
}

export function selectOfficeInboxCount(state: AppState): number {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return 0;
  return state.office.inboxCountByWorkspaceId[workspaceId] ?? 0;
}

export function selectOfficeDashboard(state: AppState): DashboardData | null {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return null;
  return state.office.dashboardByWorkspaceId[workspaceId] ?? null;
}

/** The active agent profile by id, scoped to the active workspace. */
export function selectOfficeAgentProfile(
  state: AppState,
  agentProfileId: string | null | undefined,
): AgentProfile | undefined {
  if (!agentProfileId) return undefined;
  return selectOfficeAgentProfiles(state).find((agent) => agent.id === agentProfileId);
}

/** The active project by id, scoped to the active workspace. */
export function selectOfficeProject(
  state: AppState,
  projectId: string | null | undefined,
): Project | undefined {
  if (!projectId) return undefined;
  return selectOfficeProjects(state).find((project) => project.id === projectId);
}
