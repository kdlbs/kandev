import type { ActiveSessionInfo } from "@/lib/types/agent-profile-errors";

export type WorkspaceSessionGroup = {
  workspaceId: string;
  workspaceName: string;
  sessions: ActiveSessionInfo[];
};

const compareText = (a: string, b: string) =>
  a.localeCompare(b, undefined, { sensitivity: "base" });
function compareIds(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}

function compareGroups(a: WorkspaceSessionGroup, b: WorkspaceSessionGroup): number {
  if (!a.workspaceName !== !b.workspaceName) return a.workspaceName ? -1 : 1;
  return compareText(a.workspaceName, b.workspaceName) || compareIds(a.workspaceId, b.workspaceId);
}

function compareSessions(a: ActiveSessionInfo, b: ActiveSessionInfo): number {
  return compareText(a.task_title, b.task_title) || compareIds(a.task_id, b.task_id);
}

/**
 * Groups delete-conflict sessions by workspace in a stable display order:
 * named workspaces by name then ID, unnamed workspaces last, and sessions by
 * title then task ID. A group without a response name resolves it through
 * `resolveName` before ordering, so the sort ranks the name the dialog shows;
 * response names stay authoritative.
 */
export function groupSessionsByWorkspace(
  sessions: ActiveSessionInfo[],
  resolveName: (workspaceId: string) => string | undefined = () => undefined,
): WorkspaceSessionGroup[] {
  const groups = new Map<string, WorkspaceSessionGroup>();
  for (const session of sessions) {
    const workspaceId = session.workspace_id ?? "";
    let group = groups.get(workspaceId);
    if (!group) {
      group = { workspaceId, workspaceName: "", sessions: [] };
      groups.set(workspaceId, group);
    }
    group.workspaceName ||= session.workspace_name ?? "";
    group.sessions.push(session);
  }
  for (const group of groups.values()) {
    group.workspaceName ||= resolveName(group.workspaceId) ?? "";
  }
  const ordered = [...groups.values()].sort(compareGroups);
  for (const group of ordered) group.sessions.sort(compareSessions);
  return ordered;
}
