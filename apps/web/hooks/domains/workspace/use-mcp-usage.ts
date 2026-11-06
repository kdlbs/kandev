"use client";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { listMCPUsage } from "@/lib/api/domains/mcp-api";
import { ApiError } from "@/lib/api/client";
import type { MCPOwnerUsage } from "@/lib/types/http-mcp";

type UsageState = {
  key: string;
  items: MCPOwnerUsage[];
  cursor?: string;
  busy: boolean;
  loaded: boolean;
  error: unknown;
  pageError: unknown;
  stale: boolean;
  unavailable: boolean;
};
function emptyUsage(key: string): UsageState {
  return {
    key,
    items: [],
    busy: false,
    loaded: false,
    error: null,
    pageError: null,
    stale: false,
    unavailable: false,
  };
}
function mergeOwners(current: MCPOwnerUsage[], incoming: MCPOwnerUsage[]) {
  return [
    ...new Map(
      [...current, ...incoming].map((owner) => [
        JSON.stringify([owner.scope, owner.owner_id]),
        owner,
      ]),
    ).values(),
  ];
}

export function useMCPUsage(workspaceId: string | null, definitionId: string | null) {
  const key = JSON.stringify([workspaceId, definitionId]);
  const [state, setState] = useState(() => emptyUsage(key));
  const current = state.key === key ? state : emptyUsage(key);
  const stateRef = useRef(current);
  const contextRef = useRef(key);
  useLayoutEffect(() => {
    stateRef.current = current;
    contextRef.current = key;
  });
  const requestRef = useRef(0);
  const busyRef = useRef(false);

  const load = useCallback(
    async (cursor?: string) => {
      if (!workspaceId || !definitionId || (cursor && busyRef.current)) return;
      const request = ++requestRef.current;
      const isCurrent = () => requestRef.current === request && contextRef.current === key;
      busyRef.current = true;
      setState((previous) => ({
        ...(previous.key === key ? previous : emptyUsage(key)),
        busy: true,
        pageError: null,
      }));
      try {
        const page = await listMCPUsage(workspaceId, definitionId, cursor);
        if (!isCurrent()) return;
        if (
          page.workspace_id !== workspaceId ||
          page.definition_id !== definitionId ||
          page.items.some((row) => row.workspace_id !== workspaceId)
        ) {
          // i18n-exempt: internal response identity violation; the UI renders a translated error.
          throw new Error("MCP usage response context changed");
        }
        setState((previous) => ({
          key,
          items: mergeOwners(cursor && previous.key === key ? previous.items : [], page.items),
          cursor: page.next_cursor,
          busy: false,
          loaded: true,
          error: null,
          pageError: null,
          stale: false,
          unavailable: false,
        }));
      } catch (error) {
        if (!isCurrent()) return;
        const unavailable = error instanceof ApiError && [403, 404].includes(error.status);
        setState((previous) => ({
          ...(previous.key === key && !unavailable ? previous : emptyUsage(key)),
          busy: false,
          error: cursor && !unavailable ? previous.error : error,
          pageError: cursor && !unavailable ? error : null,
          stale: !unavailable && (cursor ? previous.stale : previous.items.length > 0),
          unavailable,
        }));
      } finally {
        if (isCurrent()) busyRef.current = false;
      }
    },
    [workspaceId, definitionId, key],
  );

  useEffect(() => {
    busyRef.current = false;
    setState(emptyUsage(key));
    void load();
    return () => {
      requestRef.current += 1;
    };
  }, [key, load]);
  const reload = useCallback(() => load(), [load]);
  const loadMore = useCallback(async () => {
    const cursor = stateRef.current.cursor;
    if (cursor && !stateRef.current.stale) await load(cursor);
  }, [load]);
  return {
    items: current.items,
    loading: Boolean(workspaceId && definitionId) && !current.loaded && !current.error,
    busy: current.busy,
    hasMore: Boolean(current.cursor),
    error: current.error,
    pageError: current.pageError,
    stale: current.stale,
    unavailable: current.unavailable,
    reload,
    loadMore,
  };
}
