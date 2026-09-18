import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWebSocketClient } from "@/lib/ws/connection";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import {
  AssistantCollection,
  AssistantObservation,
} from "@/lib/orchestration/assistant-observation";
import {
  getAssistantPage,
  type AssistantBinding,
  type AssistantPages,
} from "@/lib/api/domains/assistant-api";

export function useAssistant() {
  const owner = useAppStore((s) => s.auth.user?.id);
  const connection = useAppStore((s) => s.connection.status);
  const client = useWebSocketClient();
  const view = useMemo(() => new AssistantObservation(), [owner]);
  const snapshot = useSyncExternalStore(view.subscribe, view.getSnapshot, view.getSnapshot);
  useEffect(() => {
    view.activate();
    void view.refresh();
    return view.dispose;
  }, [view]);
  useEffect(() => {
    if (connection === "connected") void view.refresh();
  }, [connection, view]);
  useEffect(
    () =>
      client?.on("orchestration.assistant.updated", () => {
        void view.refresh();
      }),
    [client, view],
  );
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (!document.hidden) void view.refresh();
    }, 30_000);
    return () => window.clearInterval(timer);
  }, [view]);
  useForegroundRefresh(view.refresh, true, view);
  return { ...snapshot, refresh: view.refresh };
}
export function useAssistantPage<K extends keyof AssistantPages>(
  kind: K,
  binding: AssistantBinding,
  revision: number,
) {
  const owner = useAppStore((s) => s.auth.user?.id);
  const identity = `${owner}:${binding.id}:${binding.version}:${binding.home_workspace_id}`;
  const view = useMemo(
    () =>
      new AssistantCollection<AssistantPages[K]>((after, signal) =>
        getAssistantPage(kind, after, signal),
      ),
    [kind, identity],
  );
  const snapshot = useSyncExternalStore(view.subscribe, view.getSnapshot, view.getSnapshot);
  useEffect(() => {
    view.activate();
    return view.dispose;
  }, [view]);
  useEffect(() => {
    void view.refresh();
  }, [view, revision]);
  return { ...snapshot, refresh: view.refresh, loadMore: view.loadMore };
}
