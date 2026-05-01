import { getWebSocketClient } from "@/lib/ws/connection";

export type SessionIntent =
  | "prepare"
  | "start"
  | "start_created"
  | "resume"
  | "workflow_step"
  | "restore_workspace";

export type MessageAttachment = {
  type: "image" | "audio" | "resource";
  data: string;
  mime_type: string;
  name?: string;
};

export type LaunchSessionRequest = {
  task_id: string;
  intent?: SessionIntent;
  session_id?: string;
  agent_profile_id?: string;
  executor_id?: string;
  executor_profile_id?: string;
  prompt?: string;
  plan_mode?: boolean;
  workflow_step_id?: string;
  priority?: number;
  launch_workspace?: boolean;
  skip_message_record?: boolean;
  auto_start?: boolean;
  attachments?: MessageAttachment[];
};

export type LaunchSessionResponse = {
  success: boolean;
  task_id: string;
  session_id?: string;
  agent_execution_id?: string;
  state: string;
  worktree_path?: string;
  worktree_branch?: string;
};

export async function launchSession(
  request: LaunchSessionRequest,
  timeout?: number,
): Promise<LaunchSessionResponse> {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  const effectiveTimeout = timeout ?? (request.intent === "resume" ? 30_000 : 15_000);
  return client.request<LaunchSessionResponse>("session.launch", request, effectiveTimeout);
}

export type EnsureSessionResponse = {
  success: boolean;
  task_id: string;
  session_id?: string;
  state: string;
  agent_profile_id?: string;
  source: "existing_primary" | "existing_newest" | "created_prepare" | "created_start";
  newly_created: boolean;
};

/**
 * Server-authoritative idempotent ensure: returns the task's existing primary
 * (or newest) session, otherwise creates one with the agent profile resolved
 * server-side from task metadata, workflow step, workflow, or workspace
 * default. Safe for preview mode — the backend chooses prepare vs start based
 * on the workflow step's auto_start_agent action.
 */
export async function ensureTaskSession(
  taskId: string,
  timeout?: number,
): Promise<EnsureSessionResponse> {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  return client.request<EnsureSessionResponse>(
    "session.ensure",
    { task_id: taskId },
    timeout ?? 15_000,
  );
}
