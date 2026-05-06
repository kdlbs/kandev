import type React from "react";
import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { setChatDraftContent } from "@/lib/local-storage";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildImplementPlanContent } from "./use-plan-actions";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";

/**
 * Launches a brand-new agent session that starts implementing the task plan
 * from a clean context window. Inherits agent + executor from the planning
 * session so the user doesn't pick anything; planning session is left running
 * in parallel. Reuses the same kandev-system block as the same-session
 * "Implement plan" path — both rely on get_task_plan_kandev to load the plan,
 * which is task-scoped.
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
  const { toast } = useToast();

  return useCallback(async () => {
    if (
      !taskId ||
      !resolvedSessionId ||
      !planningSession?.agent_profile_id ||
      !planningSession?.executor_id
    ) {
      return;
    }

    const userText = chatInputRef.current?.getValue() ?? "";
    const attachments = chatInputRef.current?.getAttachments() ?? [];

    const prompt = buildImplementPlanContent(userText);

    try {
      await launchSession({
        task_id: taskId,
        intent: "start",
        agent_profile_id: planningSession.agent_profile_id,
        executor_id: planningSession.executor_id,
        prompt,
        plan_mode: false,
        ...(attachments.length > 0 && { attachments }),
      });
      // Clear composer + draft only on success so a failed launch leaves the
      // user's input intact for retry.
      chatInputRef.current?.clear();
      setChatDraftContent(resolvedSessionId, null);
    } catch (err) {
      console.error("Failed to launch fresh implementation session:", err);
      toast({ description: "Failed to start implementation session", variant: "error" });
    }
  }, [taskId, resolvedSessionId, planningSession, chatInputRef, toast]);
}
