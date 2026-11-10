import type { MCPProfileOwner } from "@/lib/types/http-mcp";
import type { Repository, Task, TaskSession } from "@/lib/types/http";
import type { MCPOwnerDestination } from "./management-links";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";

export type MCPAssignmentTarget = Exclude<MCPOwnerDestination, { scope: "task" }> & {
  label: string;
  detail?: string;
};

type TaskSource = Pick<
  Task,
  "id" | "title" | "workspace_id" | "archived_at" | "workspace_orphaned"
>;
type SessionSource = Pick<
  TaskSession,
  | "id"
  | "task_id"
  | "state"
  | "agent_profile_id"
  | "is_passthrough"
  | "name"
  | "agent_profile_snapshot"
>;

export function profileAssignmentTargets(
  workspaceId: string,
  profiles: readonly MCPProfileOwner[],
): MCPAssignmentTarget[] {
  const seen = new Set<string>();
  return profiles.flatMap((profile) => {
    if (
      !profile.id ||
      !profile.agent_id ||
      profile.kind !== "concrete" ||
      !profile.enabled ||
      !profile.supports_mcp ||
      (profile.workspace_id && profile.workspace_id !== workspaceId) ||
      seen.has(profile.id)
    )
      return [];
    seen.add(profile.id);
    return [
      {
        scope: "profile" as const,
        workspaceId,
        ownerId: profile.id,
        agentId: profile.agent_id,
        label: profile.name || profile.id,
        detail: profile.agent_name,
      },
    ];
  });
}

export function repositoryAssignmentTargets(
  workspaceId: string,
  repositories: ReadonlyArray<Pick<Repository, "id" | "workspace_id" | "name">>,
): MCPAssignmentTarget[] {
  return repositories
    .filter((repository) => repository.workspace_id === workspaceId)
    .map((repository) => ({
      scope: "repository",
      workspaceId,
      ownerId: repository.id,
      label: repository.name || repository.id,
    }));
}

export function sessionAssignmentTargets(
  workspaceId: string,
  task: TaskSource,
  sessions: readonly SessionSource[],
  profiles: readonly MCPAssignmentTarget[],
): MCPAssignmentTarget[] {
  if (task.workspace_id !== workspaceId || task.archived_at || task.workspace_orphaned) return [];
  return sessions.flatMap((session) => {
    if (
      session.task_id !== task.id ||
      isPassthroughSession(session) ||
      ["COMPLETED", "FAILED", "CANCELLED"].includes(session.state)
    )
      return [];
    const profile = profiles.find(
      (profile) =>
        profile.scope === "profile" &&
        profile.workspaceId === workspaceId &&
        profile.ownerId === session.agent_profile_id,
    );
    if (!profile) return [];
    return [
      {
        scope: "task_session" as const,
        workspaceId,
        ownerId: session.id,
        taskId: task.id,
        label: session.name || profile.label,
        detail: `${profile.label} · ${session.id}`,
      },
    ];
  });
}
