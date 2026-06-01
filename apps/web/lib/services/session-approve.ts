import type { QueryClient } from "@tanstack/react-query";
import { approveSessionAction } from "@/app/actions/workspaces";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildWorkflowStepRequest } from "@/lib/services/session-launch-helpers";
import { mergeTaskSessionIntoCache } from "@/lib/query/cache/task-session-cache";
import type { TaskSession } from "@/lib/types/http";

/** Handle approve action: call server action and trigger auto-start if configured. */
export async function executeApprove(sessionId: string, taskId: string, queryClient: QueryClient) {
  const response = await approveSessionAction(sessionId);
  if (response?.session) {
    mergeTaskSessionIntoCache(queryClient, response.session as TaskSession);
  }
  if (
    response?.workflow_step?.events?.on_enter?.some(
      (a: { type: string }) => a.type === "auto_start_agent",
    )
  ) {
    const { request } = buildWorkflowStepRequest(taskId, sessionId, response.workflow_step.id);
    launchSession(request).catch((err) =>
      console.error("Failed to auto-start workflow step:", err),
    );
  }
}
