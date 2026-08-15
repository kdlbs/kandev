"use client";

import { useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import type { SecretScope } from "@/lib/types/http-secrets";

/**
 * Exact-trimmed name conflict helper: a copy/move target name conflicts when
 * an identical (trimmed) name already exists in the destination scope. The
 * backend remains authoritative; this only powers the dialog's pre-check.
 */
export function secretNameConflict(names: string[], name: string): boolean {
  const trimmed = name.trim();
  return trimmed !== "" && names.includes(trimmed);
}

/**
 * Loads the existing secret names of a destination scope for the copy/move
 * conflict pre-check. Global names come from the store; workspace names are
 * fetched on demand and cached per workspace.
 */
export function useSecretDestinationNames(
  scope: SecretScope,
  workspaceId?: string,
  refreshKey = 0,
) {
  const globalItems = useAppStore((state) => state.secrets.items);
  const [workspaceNames, setWorkspaceNames] = useState<string[]>([]);
  const [workspaceLoaded, setWorkspaceLoaded] = useState(scope !== "workspace");
  // Ref-based cache key: a state key would re-run the effect (and its
  // cleanup, which cancels the in-flight request) on every fetch.
  const cacheKeyRef = useRef("");
  const lastRefreshKey = useRef(refreshKey);

  useEffect(() => {
    // A new transfer session (different secret or reopened dialog) must see
    // fresh destination names, e.g. a secret copied in the previous session.
    if (refreshKey !== lastRefreshKey.current) {
      lastRefreshKey.current = refreshKey;
      cacheKeyRef.current = "";
    }
    if (scope !== "workspace") {
      return;
    }
    const key = workspaceId ?? "";
    if (key === cacheKeyRef.current) {
      return;
    }
    cacheKeyRef.current = key;
    if (!workspaceId) {
      setWorkspaceNames([]);
      setWorkspaceLoaded(true);
      return;
    }
    let cancelled = false;
    setWorkspaceLoaded(false);
    listSecrets({ scope: "workspace", workspaceId, cache: "no-store" })
      .then((items) => {
        if (!cancelled) {
          setWorkspaceNames((items ?? []).map((item) => item.name));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setWorkspaceNames([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setWorkspaceLoaded(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [scope, workspaceId, refreshKey]);

  const names =
    scope === "global"
      ? globalItems.filter((item) => item.scope !== "workspace").map((item) => item.name)
      : workspaceNames;
  const loaded = scope === "global" || workspaceLoaded;

  return {
    names,
    loaded,
    conflict: (name: string) => loaded && secretNameConflict(names, name),
  };
}
