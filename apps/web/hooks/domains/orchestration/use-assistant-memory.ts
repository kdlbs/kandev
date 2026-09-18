import { useCallback, useRef, useState } from "react";
import {
  editAssistantMemory,
  forgetAssistantMemory,
  getAssistantMemorySource,
  type AssistantBinding,
  type AssistantMemoryEdit,
} from "@/lib/api/domains/assistant-api";
import { getConversationCommentPage } from "@/lib/api/domains/orchestration-conversation-api";
import { useOrchestrationData } from "./use-orchestration-data";
export function useAssistantMemoryActions(refresh: () => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const inflight = useRef(false);
  const run = async (action: () => Promise<unknown>) => {
    if (inflight.current) return false;
    inflight.current = true;
    setBusy(true);
    setError(undefined);
    try {
      await action();
      refresh();
      return true;
    } catch (cause) {
      setError(cause);
      refresh();
      return false;
    } finally {
      inflight.current = false;
      setBusy(false);
    }
  };
  return {
    busy,
    error,
    save: (id: string, body: AssistantMemoryEdit) => run(() => editAssistantMemory(id, body)),
    forget: (id: string, revision: number) => run(() => forgetAssistantMemory(id, revision)),
  };
}
export function useAssistantMemorySource(id: string, owner: string) {
  const load = useCallback(() => getAssistantMemorySource(id), [id]);
  return useOrchestrationData(load, owner);
}
export function useAssistantMemorySources(binding: AssistantBinding) {
  const load = useCallback(
    () => getConversationCommentPage(binding.conversation_id),
    [binding.conversation_id],
  );
  const result = useOrchestrationData(
    load,
    `${binding.owner_user_id}:${binding.id}:${binding.version}`,
  );
  return {
    ...result,
    comments:
      result.data?.comments.filter(
        (row) => row.authorType === "user" && row.authorId === binding.owner_user_id,
      ) ?? [],
  };
}
