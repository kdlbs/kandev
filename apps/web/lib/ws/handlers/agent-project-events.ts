export type AgentProjectTaskEvent = {
  workspaceId: string;
  projectId: string;
};

type Listener = (event: AgentProjectTaskEvent) => void;

const listeners = new Set<Listener>();

export function subscribeAgentProjectTaskEvents(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function publishAgentProjectTaskEvent(event: {
  workspace_id?: string;
  agent_project_id?: string;
}): void {
  if (!event.workspace_id || !event.agent_project_id) return;
  const normalized = { workspaceId: event.workspace_id, projectId: event.agent_project_id };
  for (const listener of listeners) listener(normalized);
}
