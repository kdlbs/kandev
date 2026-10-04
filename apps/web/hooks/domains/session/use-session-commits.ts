import { useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { SessionCommit } from "@/lib/state/slices/session-runtime/types";
import { useSessionRead } from "./use-session-read";
import { getWebSocketClient } from "@/lib/ws/connection";

export function useSessionCommits(sessionId: string | null) {
  const store = useAppStoreApi();
  const envKey = useAppStore((state) =>
    sessionId ? (state.environmentIdBySessionId[sessionId] ?? sessionId) : null,
  );
  const commits = useAppStore((state) =>
    envKey ? state.sessionCommits.byEnvironmentId[envKey] : undefined,
  );
  const refetchTrigger = useAppStore((state) =>
    envKey ? (state.sessionCommits.refetchTrigger[envKey] ?? 0) : 0,
  );
  const { read, snapshot, scope } = useSessionRead<SessionCommit[]>(
    sessionId && envKey
      ? {
          resource: "commits",
          key: JSON.stringify([sessionId, envKey]),
          environmentId: envKey,
          sessionId,
          onLoading: (loading) => store.getState().setSessionCommitsLoading(sessionId, loading),
          fetch: async (isCurrent) => {
            const response = await getWebSocketClient()!.request<{
              commits?: SessionCommit[];
              ready?: boolean;
              reason?: string;
            }>("session.git.commits", { session_id: sessionId });
            if (!isCurrent()) return {};
            if (response?.ready === false) {
              return response.reason === "session_terminal" ? {} : { retryAfter: 2000 };
            }
            if (response?.commits) {
              // Reset/branch invalidations may authoritatively remove every commit.
              store
                .getState()
                .setSessionCommits(
                  sessionId,
                  response.commits,
                  refetchTrigger ? { allowEmpty: true } : undefined,
                );
            }
            return { data: response?.commits };
          },
        }
      : null,
  );
  useEffect(() => {
    read?.ensure(refetchTrigger, commits === undefined);
  }, [read, scope, refetchTrigger, commits]);
  return {
    commits: commits ?? [],
    loaded: snapshot.data !== undefined || commits !== undefined,
    loading: snapshot.loading,
    refetch: read?.refetch ?? noRefetch,
  };
}

async function noRefetch() {}
