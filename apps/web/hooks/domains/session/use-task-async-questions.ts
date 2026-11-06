import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listAgentQuestions } from "@/lib/api/domains/agent-questions-api";
import { taskAsyncQuestionsScopeKey } from "@/lib/state/slices/task-async-questions/scope-key";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";

export function useTaskAsyncQuestions(
  workspaceId: string | null,
  taskId: string | null,
  enabled = true,
) {
  const storeApi = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const scopeKey = workspaceId && taskId ? taskAsyncQuestionsScopeKey(workspaceId, taskId) : null;
  const scopeState = useAppStore((state) =>
    scopeKey ? state.taskAsyncQuestions.byScopeKey[scopeKey] : undefined,
  );

  const refresh = useCallback(async () => {
    if (!enabled || !workspaceId || !taskId || !scopeKey) return;
    const generation = storeApi.getState().beginTaskAsyncQuestionsRead(scopeKey);
    try {
      const page = await listAgentQuestions(workspaceId, {
        taskId,
        status: "open",
        limit: 200,
      });
      storeApi.getState().setTaskAsyncQuestionsPage(scopeKey, generation, page.questions);
    } catch {
      storeApi.getState().setTaskAsyncQuestionsError(scopeKey, generation);
    }
  }, [enabled, scopeKey, storeApi, taskId, workspaceId]);

  useEffect(() => {
    if (!enabled || !workspaceId || !taskId) return;
    void refresh();
    const client = getWebSocketClient();
    if (!client) return;
    return client.on("task.question.changed", (message) => {
      const payload = message.payload;
      if (payload.task_id === taskId || payload.parent_task_id === taskId) void refresh();
    });
  }, [enabled, refresh, taskId, workspaceId]);

  const previousConnectionStatusRef = useRef(connectionStatus);
  useEffect(() => {
    const reconnected =
      connectionStatus === "connected" && previousConnectionStatusRef.current !== "connected";
    previousConnectionStatusRef.current = connectionStatus;
    if (reconnected && enabled && workspaceId && taskId) void refresh();
  }, [connectionStatus, enabled, refresh, taskId, workspaceId]);

  useForegroundRefresh(refresh, enabled && Boolean(scopeKey), scopeKey);

  return {
    questions: scopeState?.questions ?? [],
    status: scopeState?.status ?? "idle",
    refresh,
  };
}
