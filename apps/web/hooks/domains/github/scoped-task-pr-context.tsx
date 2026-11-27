import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { deleteTaskPR, listTaskPRs } from "@/lib/api/domains/github-api";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { getWebSocketClient, useWebSocketClient } from "@/lib/ws/connection";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import type { TaskPR } from "@/lib/types/github";

type ScopedTaskPRs = {
  taskId: string;
  pr: TaskPR | null;
  prs: TaskPR[];
  loaded: boolean;
  refresh: () => Promise<void>;
  unlink: (associationId: string) => Promise<void>;
};
const Context = createContext<ScopedTaskPRs | null>(null);
const EMPTY_PRS: TaskPR[] = [];

/** An embedded original task must not republish its PRs into another workspace's cache. */
export function ScopedTaskPRProvider({
  taskId,
  workspaceId,
  children,
}: {
  taskId: string;
  workspaceId: string;
  children: ReactNode;
}) {
  const value = useScopedTaskPRs(taskId, workspaceId);
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useScopedTaskPRResult(taskId: string | null) {
  const value = useContext(Context);
  return value?.taskId === taskId ? value : null;
}

function useScopedTaskPRs(taskId: string, workspaceId: string): ScopedTaskPRs {
  const store = useAppStoreApi();
  const identity = useAppStore(stateReadScopeIdentity);
  const key = JSON.stringify([identity, workspaceId, taskId]);
  const ws = useWebSocketClient();
  const [snapshot, setSnapshot] = useState<{ key: string; prs: TaskPR[] }>();
  const request = useRef<AbortController | null>(null);
  const currentKey = useRef<string | null>(null);
  useLayoutEffect(() => {
    currentKey.current = key;
    return () => {
      currentKey.current = null;
      request.current?.abort();
    };
  }, [key]);
  const read = useCallback(async () => {
    if (currentKey.current !== key || stateReadScopeIdentity(store.getState()) !== identity) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const current = () =>
      !controller.signal.aborted &&
      currentKey.current === key &&
      stateReadScopeIdentity(store.getState()) === identity;
    try {
      const result = await listTaskPRs([taskId], {
        cache: "no-store",
        init: { signal: controller.signal },
      });
      if (!current()) return;
      const prs = (result.task_prs?.[taskId] ?? EMPTY_PRS).filter(
        (pr) => pr.task_id === taskId && pr.workspace_id === workspaceId,
      );
      setSnapshot({ key, prs });
    } catch {
      if (current()) setSnapshot(undefined);
    }
  }, [identity, key, store, taskId, workspaceId]);
  useEffect(() => {
    void read();
    return () => request.current?.abort();
  }, [read]);
  useForegroundRefresh(read, true, key);
  useEffect(() => {
    if (!ws) return;
    const updated = ws.on("github.task_pr.updated", ({ payload }) => {
      if (payload.task_id === taskId && payload.workspace_id === workspaceId) void read();
    });
    const deleted = ws.on("github.task_pr.deleted", ({ payload }) => {
      if (payload.task_id === taskId && payload.workspace_id === workspaceId) void read();
    });
    return () => {
      updated();
      deleted();
    };
  }, [read, taskId, workspaceId, ws]);
  const refresh = useCallback(async () => {
    const client = getWebSocketClient();
    if (currentKey.current !== key || stateReadScopeIdentity(store.getState()) !== identity) return;
    try {
      if (client)
        await client.request("github.task_pr.sync", { task_id: taskId, explicit_refresh: true });
    } finally {
      if (currentKey.current === key) await read();
    }
  }, [identity, key, read, store, taskId]);
  const prs = snapshot?.key === key ? snapshot.prs : EMPTY_PRS;
  const unlink = useCallback(
    async (associationId: string) => {
      if (currentKey.current !== key || stateReadScopeIdentity(store.getState()) !== identity)
        return;
      if (!prs.some((pr) => pr.id === associationId)) return;
      request.current?.abort();
      await deleteTaskPR(associationId, workspaceId);
      if (currentKey.current === key && stateReadScopeIdentity(store.getState()) === identity) {
        request.current?.abort();
        setSnapshot((previous) =>
          previous?.key === key
            ? { key, prs: previous.prs.filter((pr) => pr.id !== associationId) }
            : previous,
        );
      }
    },
    [identity, key, prs, store, workspaceId],
  );
  return { taskId, prs, pr: prs[0] ?? null, loaded: snapshot?.key === key, refresh, unlink };
}
