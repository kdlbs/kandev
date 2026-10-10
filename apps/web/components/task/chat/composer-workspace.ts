type TaskIdentity = { id: string };
type WorkflowIdentity = { id: string; workspaceId: string };
type OfficeTaskIdentity = { id: string; workspaceId: string };
type QuickChatIdentity = { sessionId: string; workspaceId: string };
type WorkflowSnapshot = { workflowId: string; tasks: readonly TaskIdentity[] };

function collectTaskWorkspaceIds(args: {
  taskId: string;
  activeWorkflowId: string | null;
  activeTasks: readonly TaskIdentity[];
  snapshots: readonly WorkflowSnapshot[];
  workflows: readonly WorkflowIdentity[];
}): Set<string> {
  const workflowIds = new Set<string>();
  if (args.activeWorkflowId && args.activeTasks.some((task) => task.id === args.taskId)) {
    workflowIds.add(args.activeWorkflowId);
  }
  for (const snapshot of args.snapshots) {
    if (snapshot.tasks.some((task) => task.id === args.taskId))
      workflowIds.add(snapshot.workflowId);
  }
  const workspaces = new Set<string>();
  for (const workflowId of workflowIds) {
    const workflow = args.workflows.find((item) => item.id === workflowId);
    if (workflow?.workspaceId) workspaces.add(workflow.workspaceId);
  }
  return workspaces;
}

export function resolveComposerWorkspaceId(args: {
  sessionId: string | null;
  taskId: string | null;
  quickChatSessions: readonly QuickChatIdentity[];
  activeWorkflowId: string | null;
  activeTasks: readonly TaskIdentity[];
  snapshots: readonly WorkflowSnapshot[];
  workflows: readonly WorkflowIdentity[];
  officeTasks?: readonly OfficeTaskIdentity[];
}): string | null {
  const quickChat = args.quickChatSessions.find((item) => item.sessionId === args.sessionId);
  if (quickChat) return quickChat.workspaceId;
  if (!args.taskId) return null;

  const workspaces = new Set<string>();
  for (const task of args.officeTasks ?? []) {
    if (task.id === args.taskId && task.workspaceId) workspaces.add(task.workspaceId);
  }

  for (const workspaceId of collectTaskWorkspaceIds({
    taskId: args.taskId,
    activeWorkflowId: args.activeWorkflowId,
    activeTasks: args.activeTasks,
    snapshots: args.snapshots,
    workflows: args.workflows,
  })) {
    workspaces.add(workspaceId);
  }

  return workspaces.size === 1 ? (workspaces.values().next().value ?? null) : null;
}
