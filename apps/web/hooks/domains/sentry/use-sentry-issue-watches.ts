"use client";

import { useEffect, useCallback, useRef, useState } from "react";
import {
  listSentryIssueWatches,
  createSentryIssueWatch,
  updateSentryIssueWatch,
  deleteSentryIssueWatch,
  triggerSentryIssueWatch,
} from "@/lib/api/domains/sentry-api";
import type {
  CreateSentryIssueWatchRequest,
  SentryIssueWatch,
  UpdateSentryIssueWatchRequest,
} from "@/lib/types/sentry";

/**
 * useSentryIssueWatches owns the Sentry-watcher list:
 *   - workspaceId: string    → fetch and operate on watches in one workspace
 *   - workspaceId: undefined → fetch every watch across all workspaces
 *   - workspaceId: null      → don't fetch
 *
 * Mirrors `useLinearIssueWatches`: update/delete/trigger pass the watch's
 * workspace id as the `workspace_id` query param so the backend can guard
 * cross-workspace mutations.
 */
export function useSentryIssueWatches(workspaceId?: string | null) {
  const [items, setItems] = useState<SentryIssueWatch[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);

  const lastScope = useRef<string | null | undefined>(undefined);
  const scope: string | null = workspaceId ?? null;

  useEffect(() => {
    if (lastScope.current !== undefined && lastScope.current !== scope) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting cached list when scope changes (incl. → null)
      setItems([]);
      setLoaded(false);
    }
    lastScope.current = scope;
  }, [scope]);

  useEffect(() => {
    if (workspaceId === null || loaded || loading) return;
    // ignore guards against a stale response landing after workspaceId changed
    // mid-flight — otherwise it would set loaded=true and block the new scope.
    let ignore = false;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- starting external fetch
    setLoading(true);
    listSentryIssueWatches(workspaceId ?? undefined, { cache: "no-store" })
      .then((res) => {
        if (ignore) return;
        setItems(res ?? []);
        setLoaded(true);
      })
      .catch(() => {
        if (ignore) return;
        setItems([]);
        setLoaded(true);
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [workspaceId, loaded, loading]);

  const create = useCallback(async (req: CreateSentryIssueWatchRequest) => {
    const watch = await createSentryIssueWatch(req);
    setItems((prev) => [...prev, watch]);
    return watch;
  }, []);

  const update = useCallback(
    async (id: string, watchWorkspaceId: string, req: UpdateSentryIssueWatchRequest) => {
      const watch = await updateSentryIssueWatch(id, watchWorkspaceId, req);
      setItems((prev) => prev.map((w) => (w.id === watch.id ? watch : w)));
      return watch;
    },
    [],
  );

  const remove = useCallback(async (id: string, watchWorkspaceId: string) => {
    await deleteSentryIssueWatch(id, watchWorkspaceId);
    setItems((prev) => prev.filter((w) => w.id !== id));
  }, []);

  const trigger = useCallback(async (id: string, watchWorkspaceId: string) => {
    return triggerSentryIssueWatch(id, watchWorkspaceId);
  }, []);

  return { items, loaded, loading, create, update, remove, trigger };
}
