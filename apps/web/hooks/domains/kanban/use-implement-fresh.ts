import type React from "react";
import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { setChatDraftContent } from "@/lib/local-storage";
import { launchSession } from "@/lib/services/session-launch-service";
import { getWebSocketClient } from "@/lib/ws/connection";
import { buildImplementPlanContent } from "./use-plan-actions";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";

async function setupFreshSession(newSessionId: string): Promise<void> {
  // Set the fresh session as primary
  const client = getWebSocketClient();
  if (client) {
    try {
      await client.request("session.set_primary", { session_id: newSessionId }, 10000);
    } catch (err) {
      console.error("Failed to set fresh session as primary:", err);
      // Continue even if set_primary fails
    }
  }
}

/**
 * Launches a brand-new agent session that starts implementing the task plan
 * from a clean context window. Inherits agent + executor from the planning
 * session so the user doesn't pick anything; planning session is left running
 * in parallel. Reuses the same kandev-system block as the same-session
 * "Implement plan" path — both rely on get_task_plan_kandev to load the plan,
 * which is task-scoped.
 *
 * The newly created session is automatically set as primary and focused (active
 * in the UI) so the user works with the fresh implementation context immediately.
 *
 * Context files from the planning session aren't forwarded — `launchSession`
 * doesn't support context_files yet, and the @ mentions inside the chat text
 * are already inlined as markdown in `userText`.
 */
export function useImplementFresh(
  resolvedSessionId: string | null,
  taskId: string | null,
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>,
) {
  const planningSession = useAppStore((s) =>
    resolvedSessionId ? s.taskSessions.items[resolvedSessionId] : undefined,
  );
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const { toast } = useToast();

  return useCallback(async () => {
    if (!taskId || !resolvedSessionId || !planningSession?.agent_profile_id) {
      return;
    }

    const userText = chatInputRef.current?.getValue() ?? "";
    const attachments = chatInputRef.current?.getAttachments() ?? [];
    const prompt = buildImplementPlanContent(userText);

    try {
      const response = await launchSession({
        task_id: taskId,
        intent: "start",
        agent_profile_id: planningSession.agent_profile_id,
        ...(planningSession.executor_id && { executor_id: planningSession.executor_id }),
        prompt,
        plan_mode: false,
        ...(attachments.length > 0 && { attachments }),
      });

      const newSessionId = response.session_id;
      if (!newSessionId) return;

      await setupFreshSession(newSessionId);
      setActiveSession(taskId, newSessionId);

      // Clear composer + draft only when a fresh session was actually created.
      chatInputRef.current?.clear();
      setChatDraftContent(resolvedSessionId, null);
    } catch (err) {
      console.error("Failed to launch fresh implementation session:", err);
      toast({ description: "Failed to start implementation session", variant: "error" });
    }
  }, [taskId, resolvedSessionId, planningSession, chatInputRef, setActiveSession, toast]);
}
