"use client";

import { useCallback, useEffect, useState } from "react";
import { listUserOrgs } from "@/lib/api/domains/github-api";
import type { GitHubOrg } from "@/lib/types/github";

export type UseGitHubOrgsResult = {
  /** The authenticated account first, then its GitHub organizations. */
  orgs: GitHubOrg[];
  loading: boolean;
  error: Error | null;
  refresh: () => void;
};

/** GitHub organizations visible to the workspace's personal GitHub identity. */
export function useGitHubOrgs(workspaceId: string | undefined): UseGitHubOrgsResult {
  const [orgs, setOrgs] = useState<GitHubOrg[]>([]);
  const [loading, setLoading] = useState(Boolean(workspaceId));
  const [error, setError] = useState<Error | null>(null);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!workspaceId) {
      setOrgs([]);
      setError(null);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    listUserOrgs(workspaceId)
      .then((response) => {
        if (!cancelled) setOrgs(response.orgs ?? []);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;
        setOrgs([]);
        setError(cause instanceof Error ? cause : new Error(String(cause)));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, version]);

  const refresh = useCallback(() => setVersion((current) => current + 1), []);
  return { orgs, loading, error, refresh };
}
