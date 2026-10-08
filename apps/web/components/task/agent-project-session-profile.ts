import type { AgentProject } from "@/lib/types/http-agent-projects";

type AgentProjectTaskIdentity = {
  id: string;
  agent_project_id?: string;
  agent_project_tier?: string;
  agent_project_profile_id?: string;
  metadata?: Record<string, unknown> | null;
};

export type AgentProjectSessionProfilePolicy =
  | { kind: "ordinary" }
  | { kind: "restricted"; profileId: string }
  | { kind: "unavailable" };

type ResolveAgentProjectSessionProfileInput = {
  taskId: string;
  task?: AgentProjectTaskIdentity | null;
  projects: readonly AgentProject[];
  projectsEnabled?: boolean;
  projectsLoaded: boolean;
  currentSessionTaskId?: string;
  currentSessionProfileId?: string;
};

function metadataString(task: AgentProjectTaskIdentity | null | undefined, key: string) {
  const value = task?.metadata?.[key];
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function taskProjectId(task: AgentProjectTaskIdentity | null | undefined) {
  return task?.agent_project_id || metadataString(task, "agent_project_id");
}

function taskTier(task: AgentProjectTaskIdentity | null | undefined) {
  return task?.agent_project_tier || metadataString(task, "agent_project_tier");
}

function taskPinnedProfileId(task: AgentProjectTaskIdentity | null | undefined) {
  return (
    task?.agent_project_profile_id || metadataString(task, "agent_project_profile_id") || undefined
  );
}

function hasProjectIdentity(task: AgentProjectTaskIdentity | null | undefined) {
  return Boolean(taskProjectId(task) || taskTier(task) || taskPinnedProfileId(task));
}

type ProjectMembership =
  | { kind: "ordinary" }
  | { kind: "invalid" }
  | { kind: "project"; project: AgentProject };

function resolveProjectMembership(
  taskId: string,
  task: AgentProjectTaskIdentity,
  projects: readonly AgentProject[],
): ProjectMembership {
  const explicitProjectId = taskProjectId(task);
  const membership = explicitProjectId
    ? projects.filter((project) => project.id === explicitProjectId)
    : projects.filter(
        (project) =>
          project.main_task_id === taskId || project.tasks.some(({ id }) => id === taskId),
      );

  if (membership.length === 1 && membership[0]) {
    return { kind: "project", project: membership[0] };
  }
  if (membership.length === 0 && !explicitProjectId) return { kind: "ordinary" };
  return { kind: "invalid" };
}

function resolvePinnedProfileId(
  taskId: string,
  task: AgentProjectTaskIdentity,
  project: AgentProject,
  currentSessionTaskId?: string,
  currentSessionProfileId?: string,
) {
  if (taskTier(task) === "coordinator" || project.main_task_id === taskId) {
    return project.coordinator_profile_id;
  }
  return (
    taskPinnedProfileId(task) ||
    (currentSessionTaskId === taskId ? currentSessionProfileId : undefined)
  );
}

export function resolveAgentProjectSessionProfile({
  taskId,
  task,
  projects,
  projectsEnabled = true,
  projectsLoaded,
  currentSessionTaskId,
  currentSessionProfileId,
}: ResolveAgentProjectSessionProfileInput): AgentProjectSessionProfilePolicy {
  if (!projectsEnabled) {
    return hasProjectIdentity(task) ? { kind: "unavailable" } : { kind: "ordinary" };
  }
  if (!task || task.id !== taskId || !projectsLoaded) return { kind: "unavailable" };

  const membership = resolveProjectMembership(taskId, task, projects);
  if (membership.kind === "ordinary") return { kind: "ordinary" };
  if (membership.kind !== "project") return { kind: "unavailable" };

  const profileId = resolvePinnedProfileId(
    taskId,
    task,
    membership.project,
    currentSessionTaskId,
    currentSessionProfileId,
  );

  return profileId ? { kind: "restricted", profileId } : { kind: "unavailable" };
}

export function filterAgentProjectSessionProfiles<T extends { id: string }>(
  profiles: readonly T[],
  policy: AgentProjectSessionProfilePolicy,
): T[] {
  if (policy.kind === "ordinary") return [...profiles];
  if (policy.kind === "unavailable") return [];
  return profiles.filter((profile) => profile.id === policy.profileId);
}
