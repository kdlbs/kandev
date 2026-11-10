import type { MCPSelectionOrigin } from "@/lib/types/http-mcp";

export type MCPOriginContext = {
  workspaceId: string | null;
  profiles: readonly { id: string; label: string; workspace_id?: string | null }[];
  repositories: Record<string, readonly { id: string; name: string; workspace_id: string }[]>;
  tasks: readonly { id: string; title: string; workspaceId?: string; workflowId: string }[];
  workflows: readonly { id: string; workspaceId: string }[];
  sessions: Record<string, { name?: string; task_id: string }>;
};

function taskInWorkspace(id: string, workspaceId: string, context: MCPOriginContext) {
  return context.tasks.find((task) => {
    if (task.id !== id) return false;
    const owner =
      task.workspaceId ??
      context.workflows.find((flow) => flow.id === task.workflowId)?.workspaceId;
    return owner === workspaceId;
  });
}

function originName(origin: MCPSelectionOrigin, context: MCPOriginContext): string | undefined {
  const { owner_id, workspace_id } = origin;
  switch (origin.scope) {
    case "profile":
      return context.profiles.find(
        (profile) =>
          profile.id === owner_id &&
          (!profile.workspace_id || profile.workspace_id === workspace_id),
      )?.label;
    case "repository":
      return context.repositories[workspace_id]?.find(
        (repo) => repo.id === owner_id && repo.workspace_id === workspace_id,
      )?.name;
    case "task":
      return taskInWorkspace(owner_id, workspace_id, context)?.title;
    case "task_session": {
      const session = context.sessions[owner_id];
      return session && taskInWorkspace(session.task_id, workspace_id, context)
        ? session.name
        : undefined;
    }
  }
}

export function formatMCPOriginLabel(
  origin: MCPSelectionOrigin,
  scopeLabel: string,
  context: MCPOriginContext,
): string {
  if (origin.workspace_id !== context.workspaceId) return scopeLabel;
  const name = originName(origin, context)?.trim();
  return name ? `${scopeLabel}: ${name}` : scopeLabel;
}
