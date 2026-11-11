import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getBuiltinMCPSession } from "@/lib/api/domains/builtin-mcp-api";
import type { BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import { getBackendConfig } from "@/lib/config";

export function useBuiltinMCPSession(
  taskId: string | null,
  sessionId: string | null,
  workspaceId: string,
  enabled: boolean,
) {
  const history = useAppStore((state) =>
    sessionId ? state.sessionMcpStatus.bySessionId[sessionId] : undefined,
  );
  const auth = useAppStore((state) =>
    JSON.stringify([state.auth.mode, state.auth.authenticated, state.auth.user?.id]),
  );
  const key = JSON.stringify([
    taskId,
    sessionId,
    workspaceId,
    auth,
    getBackendConfig().apiBaseUrl,
    history?.current.execution_id,
    history?.current.updated_at,
  ]);
  const current = useRef({ key, sequence: 0 });
  if (current.current.key !== key) current.current = { key, sequence: 0 };
  const [loaded, setLoaded] = useState<{
    key: string;
    view?: BuiltinMCPSessionView;
    error?: unknown;
  }>();
  const refresh = useCallback(async () => {
    if (!enabled || !taskId || !sessionId) return;
    const captured = current.current;
    const sequence = ++captured.sequence;
    const live = () => current.current === captured && captured.sequence === sequence;
    try {
      const view = await getBuiltinMCPSession(sessionId);
      if (!live()) return;
      if (
        view.task_id !== taskId ||
        view.session_id !== sessionId ||
        view.workspace_id !== workspaceId
      )
        return;
      setLoaded({ key, view });
    } catch (error) {
      if (live()) setLoaded({ key, error });
    }
  }, [enabled, key, taskId, sessionId, workspaceId]);
  useEffect(() => {
    void refresh();
    return () => {
      current.current.sequence += 1;
    };
  }, [refresh]);
  return {
    view: enabled && loaded?.key === key ? loaded.view : undefined,
    error: loaded?.key === key ? loaded.error : undefined,
    refresh,
  };
}
