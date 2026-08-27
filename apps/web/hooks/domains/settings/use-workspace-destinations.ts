"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { listWorkspaces } from "@/lib/api/domains/workspace-api";
import { mapWorkspaceItem } from "@/lib/routing/route-bootstrap";

/**
 * Ensures the workspace destination list is available to the copy/move
 * dialog with a real retry path and a single source of truth. The picker
 * reads the store only; this hook fetches `listWorkspaces` when the store's
 * list is empty (e.g. settings hydration failed) and writes successful
 * results into the store via `setWorkspaces`. Failures are preserved as
 * `error` (never converted into an empty list), and `retry` re-runs the
 * fetch. A newer hydration simply supersedes the fallback result.
 */
export function useWorkspaceDestinations() {
  const workspaceItems = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (workspaceItems.length > 0) {
      // Hydration (or a retry result) superseded the fallback request: clear
      // any in-flight loading state so submission is not disabled forever.
      setError(null);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    listWorkspaces({ cache: "no-store" })
      .then((response) => {
        if (!cancelled) {
          setWorkspaces(response.workspaces.map(mapWorkspaceItem));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setError("failed");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceItems.length, attempt, setWorkspaces]);

  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  return { loading, error, retry };
}
