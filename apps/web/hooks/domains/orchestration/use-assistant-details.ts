import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import { getAssistantCredentials, type AssistantBinding } from "@/lib/api/domains/assistant-api";
import { getConversationCommentPage } from "@/lib/api/domains/orchestration-conversation-api";
import type { TaskComment } from "@/components/task/simple/types";
import { AssistantCollection } from "@/lib/orchestration/assistant-observation";
import { useOrchestrationData } from "./use-orchestration-data";
export function useAssistantCredentials(binding: AssistantBinding, revision: number) {
  const load = useCallback(() => getAssistantCredentials(), []);
  const state = useOrchestrationData(
    load,
    `${binding.owner_user_id}:${binding.id}:${binding.version}`,
  );
  useEffect(() => {
    state.refresh();
  }, [revision, state.refresh]);
  return state;
}
export function useAssistantActivity(binding: AssistantBinding, revision: number) {
  const key = `${binding.owner_user_id}:${binding.id}:${binding.version}`;
  const id = binding.conversation_id;
  const view = useMemo(
    () =>
      new AssistantCollection<TaskComment>(async (after, signal) => {
        const page = await getConversationCommentPage(id, after, signal);
        return { entries: page.comments, next_cursor: page.next_cursor };
      }),
    [id, key],
  );
  const state = useSyncExternalStore(view.subscribe, view.getSnapshot, view.getSnapshot);
  useEffect(() => {
    view.activate();
    return view.dispose;
  }, [view]);
  useEffect(() => {
    void view.refresh();
  }, [view, revision]);
  return { ...state, refresh: view.refresh, loadMore: view.loadMore };
}
