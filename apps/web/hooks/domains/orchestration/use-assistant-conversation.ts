import { useCallback, useEffect, useMemo } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getConversation,
  getConversationCommentPage,
  createConversationSender,
} from "@/lib/api/domains/orchestration-conversation-api";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type { TaskComment } from "@/components/task/simple/types";
import { useSessionLiveSyncSubscriptions } from "@/hooks/domains/session/use-session-live-sync";
import { AssistantCollection } from "@/lib/orchestration/assistant-observation";
import { useSyncExternalStore } from "react";
import { useOrchestrationData } from "./use-orchestration-data";
export function useAssistantConversation(binding: AssistantBinding, revision: number) {
  const store = useAppStoreApi();
  const owner = useAppStore((s) => s.auth.user?.id);
  const connection = useAppStore((s) => s.connection.status);
  const id = binding.conversation_id;
  const identity = `${owner}:${binding.id}:${binding.version}:${id}`;
  const view = useMemo(
    () =>
      new AssistantCollection<TaskComment>(async (after, signal) => {
        const page = await getConversationCommentPage(id, after, signal);
        return { entries: page.comments, next_cursor: page.next_cursor };
      }),
    [id, identity],
  );
  const snapshot = useSyncExternalStore(view.subscribe, view.getSnapshot, view.getSnapshot);
  const load = useCallback(async () => {
    const activityEpochs = { ...store.getState().taskSessions.activityEpochBySession };
    const [task, result] = await Promise.all([getConversation(id), listTaskSessions(id)]);
    return { task, sessions: result.sessions ?? [], activityEpochs };
  }, [id, store]);
  const metadata = useOrchestrationData(load, identity);
  useEffect(() => {
    if (metadata.data)
      store
        .getState()
        .setTaskSessionsForTask(id, metadata.data.sessions, metadata.data.activityEpochs);
  }, [id, metadata.data, store]);
  const refresh = useCallback(async () => {
    metadata.refresh();
    await view.refresh();
  }, [metadata.refresh, view]);
  useEffect(() => {
    view.activate();
    return view.dispose;
  }, [view]);
  useEffect(() => {
    void view.refresh();
  }, [view, revision, connection]);
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (!document.hidden) {
        metadata.refresh();
        void view.refresh();
      }
    }, 3000);
    return () => window.clearInterval(timer);
  }, [view, metadata.refresh]);
  const sessionIds = metadata.data?.sessions.map((session) => session.id) ?? [];
  useSessionLiveSyncSubscriptions({ connectionStatus: connection, taskId: id, sessionIds });
  const transport = useMemo(() => createConversationSender(id), [id, identity]);
  const comments = useMemo(
    () =>
      snapshot.entries
        .slice()
        .sort((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id)),
    [snapshot.entries],
  );
  return {
    ...snapshot,
    comments,
    metadata: metadata.data,
    error: snapshot.error || metadata.error,
    transport,
    refresh,
    loadMore: view.loadMore,
  };
}
