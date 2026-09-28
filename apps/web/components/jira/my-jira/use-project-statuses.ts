"use client";

import { useEffect, useRef, useState } from "react";
import { listJiraProjectStatuses } from "@/lib/api/domains/jira-api";
import type { JiraStatus } from "@/lib/types/jira";

// reconcileStatuses drops any selected status names that are no longer present
// in the available union (e.g. after the project selection changed). Kept pure
// so it is trivially unit-testable and reusable from the page hook.
export function reconcileStatuses(selected: string[], available: JiraStatus[]): string[] {
  if (selected.length === 0) return selected;
  const names = new Set(available.map((s) => s.name));
  const next = selected.filter((name) => names.has(name));
  // Preserve referential equality when nothing changed so callers can skip
  // redundant state updates.
  return next.length === selected.length ? selected : next;
}

// Saved custom JQL owns the complete query, so its structured status snapshot
// must not be reconciled against the available status options.
export function reconcileStatusesForQuery(
  statusesLoaded: boolean,
  customJql: string | null,
  selected: string[],
  available: JiraStatus[],
  statusesAuthoritative = true,
): string[] {
  return statusesLoaded && statusesAuthoritative && customJql === null
    ? reconcileStatuses(selected, available)
    : selected;
}

// unionByName merges status lists from several projects, de-duping by name.
// Two projects may expose a status with the same name but different ids; the
// filter targets status names in JQL, so name is the identity that matters.
function unionByName(lists: JiraStatus[][]): JiraStatus[] {
  const seen = new Set<string>();
  const out: JiraStatus[] = [];
  for (const list of lists) {
    for (const s of list) {
      if (seen.has(s.name)) continue;
      seen.add(s.name);
      out.push(s);
    }
  }
  return out;
}

// ProjectStatuses is the result of useProjectStatuses. `loaded` is false while
// the fetch for the current project-key set is still pending and flips to true
// only once every selected key has resolved (from cache or network). Callers
// that reconcile a saved status selection against `options` must wait for
// `loaded`, otherwise the first render (options still []) would strip the
// selection before the statuses arrive.
export type ProjectStatuses = {
  options: JiraStatus[];
  loaded: boolean;
  authoritative: boolean;
};

type CachedProjectStatuses = {
  options: JiraStatus[];
  authoritative: boolean;
};

// useProjectStatuses fetches the workflow statuses for the selected project
// keys, unions and de-dupes them by name, and caches per project key for the
// lifetime of the component so re-selecting a project never refetches. A fetch
// failure is non-fatal, but the incomplete union is not authoritative for
// pruning saved status filters.
export function useProjectStatuses(
  projectKeys: string[],
  workspaceId?: string | null,
): ProjectStatuses {
  const [options, setOptions] = useState<JiraStatus[]>([]);
  const [loadedKey, setLoadedKey] = useState<string | null>(null);
  const cacheRef = useRef<Map<string, CachedProjectStatuses>>(new Map());

  const workspaceKey = workspaceId?.trim() ?? "";
  const projectKeySet = [...projectKeys].sort().join(",");
  const cacheKey = `${workspaceKey}|${projectKeySet}`;

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (projectKeys.length === 0) {
        setOptions([]);
        setLoadedKey(cacheKey);
        return;
      }
      const cache = cacheRef.current;
      const statusCacheKey = (key: string) => `${workspaceKey}\u0000${key}`;
      await Promise.all(
        projectKeys
          .filter((key) => !cache.has(statusCacheKey(key)))
          .map(async (key) => {
            try {
              const { statuses } = await listJiraProjectStatuses(key, {
                workspaceId: workspaceKey || undefined,
              });
              cache.set(statusCacheKey(key), { options: statuses ?? [], authoritative: true });
            } catch {
              // Keep failed lookups cached without treating the empty result as authoritative.
              cache.set(statusCacheKey(key), { options: [], authoritative: false });
            }
          }),
      );
      if (cancelled) return;
      setOptions(
        unionByName(projectKeys.map((key) => cache.get(statusCacheKey(key))?.options ?? [])),
      );
      setLoadedKey(cacheKey);
    }
    void load();
    return () => {
      cancelled = true;
    };
    // cacheKey encodes the sorted key set; projectKeys identity is unstable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cacheKey]);

  const loaded = loadedKey === cacheKey;
  const authoritative =
    loaded &&
    projectKeys.every((key) => cacheRef.current.get(`${workspaceKey}\u0000${key}`)?.authoritative);

  return { options, loaded, authoritative };
}
