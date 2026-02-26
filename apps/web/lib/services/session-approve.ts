import { approveSessionAction } from "@/app/actions/workspaces";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildWorkflowStepRequest } from "@/lib/services/session-launch-helpers";

/** Handle approve action: call server action and trigger auto-start if configured. */
export async function executeApprove(
  sessionId: string,
  taskId: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  setTaskSession: (session: any) => void,
) {
  const response = await approveSessionAction(sessionId);
  if (response?.session) {
    setTaskSession(response.session);
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
