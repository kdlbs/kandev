import { useCallback, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { toast } from "@/lib/toast/sonner";
import {
  openOrchestratorConversation,
  orchestratorConversationHref,
  listOrchestrators,
} from "@/lib/api/domains/orchestration-api";
import { useOrchestrationData } from "./use-orchestration-data";

export function useWorkspaceOrchestrators(workspaceId: string) {
  const load = useCallback(() => listOrchestrators(workspaceId), [workspaceId]);
  return useOrchestrationData(load);
}

export function useOrchestratorConversation(
  workspaceId: string,
  id: string,
  onNavigate?: () => void,
) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const open = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const conversation = await openOrchestratorConversation(workspaceId, id);
      router.push(orchestratorConversationHref(workspaceId, id, conversation.task_id));
      onNavigate?.();
    } catch (error) {
      toast.error(String(error));
    } finally {
      setBusy(false);
    }
  };
  return { open, busy };
}
