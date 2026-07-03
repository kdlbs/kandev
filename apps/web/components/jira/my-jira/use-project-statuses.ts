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
};

// useProjectStatuses fetches the workflow statuses for the selected project
// keys, unions and de-dupes them by name, and caches per project key for the
// lifetime of the component so re-selecting a project never refetches. A fetch
// failure for one project is non-fatal: that project contributes no options
// and the rest still load.
export function useProjectStatuses(projectKeys: string[]): ProjectStatuses {
  const [options, setOptions] = useState<JiraStatus[]>([]);
  const [loaded, setLoaded] = useState(false);
  const cacheRef = useRef<Map<string, JiraStatus[]>>(new Map());

  const cacheKey = [...projectKeys].sort().join(",");

  useEffect(() => {
    let cancelled = false;
    // Re-fetching for a new key set: options for the previous set are stale and
    // the current set has not resolved yet, so mark unloaded until it does.
    setLoaded(false);
    async function load() {
      if (projectKeys.length === 0) {
        setOptions([]);
        setLoaded(true);
        return;
      }
      const cache = cacheRef.current;
      await Promise.all(
        projectKeys
          .filter((key) => !cache.has(key))
          .map(async (key) => {
            try {
              const { statuses } = await listJiraProjectStatuses(key);
              cache.set(key, statuses ?? []);
            } catch {
              // Non-fatal: cache an empty list so we don't refetch on every render.
              cache.set(key, []);
            }
          }),
      );
      if (cancelled) return;
      setOptions(unionByName(projectKeys.map((key) => cache.get(key) ?? [])));
      setLoaded(true);
    }
    void load();
    return () => {
      cancelled = true;
    };
    // cacheKey encodes the sorted key set; projectKeys identity is unstable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cacheKey]);

  return { options, loaded };
}
