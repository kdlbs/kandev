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
      // Reset cached list on scope change (incl. → null). Also clear `loading`:
      // a fetch from the old scope can no longer complete into the new one (its
      // .finally is gated by `ignore`), and a → null scope starts no replacement
      // fetch, so without this the hook would stay stuck at loading=true.
      /* eslint-disable react-hooks/set-state-in-effect -- resetting cached state when scope changes */
      setItems([]);
      setLoaded(false);
      setLoading(false);
      /* eslint-enable react-hooks/set-state-in-effect */
    }
    lastScope.current = scope;
  }, [scope]);

  useEffect(() => {
    if (workspaceId === null || loaded) return;
    // `loading` is intentionally NOT a dependency: setLoading(true) below would
    // otherwise re-run this effect, whose cleanup sets ignore=true on the
    // in-flight request, so its .finally skips setLoading(false) and the hook
    // sticks at loading=true forever. The `loaded` guard already prevents a
    // duplicate fetch, and `ignore` handles a workspaceId change mid-flight.
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
  }, [workspaceId, loaded]);

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
