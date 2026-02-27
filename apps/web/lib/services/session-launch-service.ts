import { getWebSocketClient } from "@/lib/ws/connection";

export type SessionIntent = "prepare" | "start" | "start_created" | "resume" | "workflow_step" | "restore_workspace";

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
