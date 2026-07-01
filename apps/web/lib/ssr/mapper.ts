import type { QuerySeedInitialState } from "@/lib/query/seed";
import type { WorkflowSnapshot, Message, Task } from "@/lib/types/http";

export function snapshotToState(snapshot: WorkflowSnapshot): QuerySeedInitialState {
  // Handle empty snapshot (ephemeral tasks have no workflow)
  if (!snapshot.workflow) {
    return {};
  }
  return {
    workflowSnapshots: {
      itemsByWorkflowId: {
        [snapshot.workflow.id]: snapshot,
      },
    },
  };
}

export function taskToState(
  task: Task,
  sessionId?: string | null,
  messages?: { items: Message[]; hasMore?: boolean; oldestCursor?: string | null },
): QuerySeedInitialState {
  const resolvedSessionId = sessionId ?? messages?.items[0]?.session_id ?? null;
  return {
    tasks: {
      activeTaskId: task.id,
      activeSessionId: resolvedSessionId,
      pinnedSessionId: null,
      lastSessionByTaskId: resolvedSessionId ? { [task.id]: resolvedSessionId } : {},
    },
    messages:
      resolvedSessionId && messages
        ? {
            bySession: {
              [resolvedSessionId]: messages.items,
            },
            metaBySession: {
              [resolvedSessionId]: {
                isLoading: false,
                hasMore: messages.hasMore ?? false,
                oldestCursor: messages.oldestCursor ?? messages.items[0]?.id ?? null,
              },
            },
          }
        : undefined,
  };
}
