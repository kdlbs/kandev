"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getRepoMergeAccess } from "@/lib/api/domains/github-api";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import type { GitHubStatus, RepoMergeAccess } from "@/lib/types/github";
import { useGitHubStatus } from "./use-github-status";

export type MergeAccessSnapshot = Omit<RepoMergeAccess, "state"> & {
  state: RepoMergeAccess["state"] | "loading";
};

function credentialKey(status: GitHubStatus | null) {
  return [
    status?.authenticated,
    status?.effective_manual_mutation_actor,
    status?.automation?.status,
    status?.automation?.credential_generation,
    status?.personal?.status,
    status?.personal?.credential_generation,
  ];
}

export function useRepoMergeAccess(
  workspaceId: string | null,
  owner: string,
  repo: string,
  enabled = true,
  expectedHost = "github.com",
) {
  const { status } = useGitHubStatus(workspaceId);
  const auth = useAppStore((state) => state.auth);
  const key = JSON.stringify([
    workspaceId,
    owner,
    repo,
    expectedHost,
    enabled,
    auth.user?.id,
    auth.mode,
    auth.authenticated,
    credentialKey(status),
  ]);
  const currentKey = useRef(key);
  useLayoutEffect(() => {
    currentKey.current = key;
  }, [key]);
  const generation = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const [snapshot, setSnapshot] = useState<(MergeAccessSnapshot & { key: string }) | null>(null);

  const refresh = useCallback(() => {
    if (!enabled || !workspaceId || !owner || !repo || currentKey.current !== key) return;
    controller.current?.abort();
    const request = ++generation.current;
    const abort = new AbortController();
    controller.current = abort;
    setSnapshot({ key, state: "loading", actor: null });
    const isCurrent = () => request === generation.current && currentKey.current === key;
    void getRepoMergeAccess(workspaceId, owner, repo, {
      expectedHost,
      init: { signal: abort.signal },
    })
      .then((access) => {
        if (!isCurrent()) return;
        if (!access.actor || access.actor.workspace_id !== workspaceId) {
          setSnapshot({ key, state: "unknown", actor: null });
          return;
        }
        setSnapshot({ key, ...access });
      })
      .catch(() => {
        if (isCurrent()) setSnapshot({ key, state: "unknown", actor: null });
      });
  }, [enabled, workspaceId, owner, repo, key, expectedHost]);

  useEffect(() => {
    refresh();
    const unsubscribe = subscribeIntegrationAvailability(refresh);
    const onFocus = () => {
      if (document.visibilityState === "visible") refresh();
    };
    window.addEventListener("focus", onFocus);
    return () => {
      generation.current++;
      controller.current?.abort();
      unsubscribe();
      window.removeEventListener("focus", onFocus);
      setSnapshot(null);
    };
  }, [refresh]);

  // A context change revokes the old grant before its replacement effect runs.
  const access: MergeAccessSnapshot =
    snapshot?.key === key
      ? snapshot
      : { state: enabled && workspaceId ? "loading" : "unknown", actor: null };
  return { ...access, refresh };
}
