import type { ApiRequestOptions } from "@/lib/api/client";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import {
  listQuickChatSessions,
  type QuickChatSessionResponse,
} from "@/lib/api/domains/workspace-api";
import type { Task } from "@/lib/types/http";
import type { ControlWorkspace } from "./use-integrated-control-data";
import { controlReadFailure } from "./use-control-resource";

export type ControlCollection<T> = {
  items: T[];
  error: boolean;
  denied: boolean;
  transient?: boolean;
  receivedAt?: string;
  stale?: boolean;
};
export type ControlTaskCount = {
  total: number | null;
  error: boolean;
  denied: boolean;
  items?: Task[];
  transient?: boolean;
  receivedAt?: string;
  stale?: boolean;
};
export type ControlConversationLists = {
  tasks?: ControlTaskCount;
  independent: ControlCollection<QuickChatSessionResponse>;
};

async function readCollection<T>(load: () => Promise<T[]>): Promise<ControlCollection<T>> {
  try {
    return { items: await load(), error: false, denied: false };
  } catch (error) {
    const status = controlReadFailure(error);
    return { items: [], error: true, denied: status === "denied", transient: status === "error" };
  }
}

export async function readControlConversationLists(
  workspaces: ControlWorkspace[],
  signal: AbortSignal,
): Promise<Record<string, ControlConversationLists>> {
  const rows = await Promise.all(
    workspaces.map(async (workspace) => {
      const options = { cache: "no-store" as const, init: { signal } };
      const [independent, tasks] = await Promise.all([
        readCollection(async () => {
          const result = await listQuickChatSessions(workspace.id, options);
          return result.sessions.filter(
            (item) => item.workspace_id === workspace.id && item.kind === "chat",
          );
        }),
        readControlTaskCount(workspace.id, options),
      ]);
      return [workspace.id, { independent, tasks }] as const;
    }),
  );
  return Object.fromEntries(rows);
}

async function readControlTaskCount(
  workspaceId: string,
  options: ApiRequestOptions,
): Promise<ControlTaskCount> {
  try {
    const { total, tasks } = await listTasksByWorkspace(
      workspaceId,
      { page: 1, pageSize: 20, includeArchived: false },
      options,
    );
    if (!Number.isSafeInteger(total) || total < 0)
      return { total: null, error: true, denied: false };
    return {
      total,
      items: tasks.filter((task) => task.workspace_id === workspaceId),
      error: false,
      denied: false,
    };
  } catch (error) {
    const status = controlReadFailure(error);
    return { total: null, error: true, denied: status === "denied", transient: status === "error" };
  }
}

// The existing task API excludes archived and ephemeral conversations by default.
// Missing/failed workspace observations cannot be presented as a complete sum.
export function controlTaskTotal(
  workspaces: ReadonlyArray<Pick<ControlWorkspace, "id">>,
  lists: Record<string, ControlConversationLists>,
): number | null {
  let total = 0;
  for (const id of new Set(workspaces.map((workspace) => workspace.id))) {
    const count = lists[id]?.tasks;
    if (!count || count.error || count.denied || count.total === null) return null;
    total += count.total;
    if (!Number.isSafeInteger(total)) return null;
  }
  return total;
}
