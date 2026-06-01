import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { gitStatusQueryOptions } from "@/lib/query/query-options/session-runtime";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

/**
 * Build a `{ environmentId -> GitStatusEntry }` map for the sidebar / mobile
 * sheet / recent-task-switcher git indicators by reading the TanStack Query git
 * cache (`qk.session.git(envKey)`) for every session's resolved environment key.
 *
 * Replaces the Zustand `gitStatus.byEnvironmentId` map read. The bridge
 * populates these keys from the workspace git-status WS stream; here we only
 * OBSERVE the cache (`enabled: false`) — issuing a fetch per session would be
 * wasteful and the bridge's `setQueryData` still notifies disabled observers.
 *
 * `environmentIdBySessionId` stays in Zustand (client-side index); we resolve
 * each session's envKey from it so the returned map is keyed by environment ID,
 * matching the previous shape its consumers index into.
 */
export function useGitStatusByEnvFromCache(sessionIds: string[]): Record<string, GitStatusEntry> {
  const envBySessionId = useAppStore((state) => state.environmentIdBySessionId);

  // Resolve each session to its environment key, de-duplicated so sessions
  // sharing an environment only register one observer.
  const envKeys = useMemo(() => {
    const set = new Set<string>();
    for (const sid of sessionIds) {
      set.add(envBySessionId[sid] ?? sid);
    }
    return [...set];
  }, [sessionIds, envBySessionId]);

  return useQueries({
    queries: envKeys.map((envKey) => ({
      ...gitStatusQueryOptions(envKey),
      enabled: false,
    })),
    combine: (results): Record<string, GitStatusEntry> => {
      const map: Record<string, GitStatusEntry> = {};
      results.forEach((result, index) => {
        const status = result.data?.byEnvironmentId;
        if (status) map[envKeys[index]] = status;
      });
      return map;
    },
  });
}
