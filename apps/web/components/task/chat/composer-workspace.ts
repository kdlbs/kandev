type TaskIdentity = { id: string };
type WorkflowIdentity = { id: string; workspaceId: string };
type QuickChatIdentity = { sessionId: string; workspaceId: string };
type WorkflowSnapshot = { workflowId: string; tasks: readonly TaskIdentity[] };

export function resolveComposerWorkspaceId(args: {
  sessionId: string | null;
  taskId: string | null;
  quickChatSessions: readonly QuickChatIdentity[];
  activeWorkflowId: string | null;
  activeTasks: readonly TaskIdentity[];
  snapshots: readonly WorkflowSnapshot[];
  workflows: readonly WorkflowIdentity[];
}): string | null {
  const quickChat = args.quickChatSessions.find((item) => item.sessionId === args.sessionId);
  if (quickChat) return quickChat.workspaceId;
  if (!args.taskId) return null;

  const activeWorkflowOwnsTask = args.activeTasks.some((task) => task.id === args.taskId);
  const workflowId = activeWorkflowOwnsTask
    ? args.activeWorkflowId
    : (args.snapshots.find((snapshot) => snapshot.tasks.some((task) => task.id === args.taskId))
        ?.workflowId ?? null);
  if (!workflowId) return null;
  return args.workflows.find((workflow) => workflow.id === workflowId)?.workspaceId ?? null;
}
