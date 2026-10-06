import { useEffect, useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";
import type {
  GitStatusEntry,
  GitStatusRefreshState,
} from "@/lib/state/slices/session-runtime/types";
import { projectGitStatusForDisplay } from "@/lib/state/slices/session-runtime/git-status-display-state";

const debugSub = createDebugLogger("git-status:subscribe");

/**
 * Hook to get the current git status for a session.
 * Git status is keyed by environment ID so sessions sharing an environment share git state.
 *
 * For multi-repo workspaces this returns whichever repo's status arrived last;
 * use useSessionGitStatusByRepo when the caller needs all repos at once.
 */
export function useSessionGitStatus(sessionId: string | null) {
  const gitStatus = useAppStore(
    useShallow((state) => {
      if (!sessionId) return undefined;
      const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
      return state.gitStatus.byEnvironmentId[envKey];
    }),
  );
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Subscribe to session updates to receive git status via WebSocket
  // The workspace stream sends current git status immediately on subscription
  useEffect(() => {
    if (!sessionId) {
      debugSub("skip", { reason: "no-session-id", connectionStatus });
      return;
    }

    // Wait for WebSocket to be connected before subscribing
    if (connectionStatus !== "connected") {
      debugSub("skip", { sessionId, reason: "not-connected", connectionStatus });
      return;
    }

    const client = getWebSocketClient();
    if (!client) {
      debugSub("skip", { sessionId, reason: "no-client", connectionStatus });
      return;
    }
    debugSub("subscribe", { sessionId, connectionStatus });
    const unsubscribe = client.subscribeSession(sessionId);
    return () => {
      debugSub("unsubscribe", { sessionId });
      unsubscribe();
      // Don't clear git status on cleanup - keep it cached for when user switches back
    };
  }, [sessionId, connectionStatus]);

  return gitStatus;
}

/**
 * Hook to get per-repository git statuses for a multi-repo session.
 * Returns an array of { repository_name, status } sorted by repo name.
 *
 * For single-repo workspaces returns a single-element array (or empty when
 * no status has arrived yet). The Changes panel uses this to merge files
 * from all repos and tag each with its repository, so the file tree's
 * existing per-repo grouping (Phase 6) kicks in automatically.
 */
export function useSessionGitStatusByRepo(
  sessionId: string | null,
): Array<{ repository_name: string; status: GitStatusEntry }> {
  const map = useAppStore(
    useShallow((state) => {
      if (!sessionId) return undefined;
      const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
      return state.gitStatus.byEnvironmentRepo[envKey];
    }),
  );
  return useMemo(() => {
    if (!map) return [];
    return Object.entries(map)
      .map(([name, status]) => ({ repository_name: name, status }))
      .sort((a, b) => a.repository_name.localeCompare(b.repository_name));
  }, [map]);
}

/** Returns raw snapshots and their separate, scope-checked display projection. */
export function useSessionGitStatusSnapshots(sessionId: string | null) {
  const gitStatus = useSessionGitStatus(sessionId);
  const statusByRepo = useSessionGitStatusByRepo(sessionId);
  const projection = useAppStore(
    useShallow((state) => {
      const environmentId = sessionId
        ? (state.environmentIdBySessionId[sessionId] ?? sessionId)
        : null;
      return {
        environmentId,
        displayByRepo: environmentId
          ? state.gitStatusDisplay.byEnvironmentRepo[environmentId]
          : undefined,
        checkoutByRepo: environmentId
          ? state.gitCheckoutGeneration.byEnvironmentId[environmentId]
          : undefined,
        environmentRefresh: environmentId
          ? state.gitStatus.refreshByEnvironmentId?.[environmentId]
          : undefined,
        refreshByRepo: environmentId
          ? state.gitStatus.refreshByEnvironmentRepo?.[environmentId]
          : undefined,
      };
    }),
  );
  const displayGitStatus = useMemo(() => {
    if (!gitStatus) return gitStatus;
    const repositoryName = gitStatus.repository_name ?? "";
    return projectGitStatusForDisplay(
      gitStatus,
      projection.displayByRepo?.[repositoryName],
      projection.checkoutByRepo?.[repositoryName] ?? 0,
      displayRefreshState(
        gitStatus,
        projection.refreshByRepo?.[repositoryName],
        projection.environmentRefresh,
      ),
    );
  }, [gitStatus, projection]);
  const displayScopeByRepo = useMemo(
    () =>
      Object.fromEntries(
        Object.entries(projection.displayByRepo ?? {}).map(([repositoryName, display]) => [
          repositoryName,
          JSON.stringify([
            projection.environmentId,
            display.checkoutGeneration,
            display.branch,
            display.headCommit,
            display.baseCommit,
            display.comparisonTarget,
          ]),
        ]),
      ),
    [projection.displayByRepo, projection.environmentId],
  );
  const displayStatusByRepo = useMemo(
    () =>
      statusByRepo.map(({ repository_name, status }) => ({
        repository_name,
        status: projectGitStatusForDisplay(
          status,
          projection.displayByRepo?.[repository_name],
          projection.checkoutByRepo?.[repository_name] ?? 0,
          displayRefreshState(
            status,
            projection.refreshByRepo?.[repository_name],
            projection.environmentRefresh,
          ),
        ),
      })),
    [statusByRepo, projection],
  );
  return { gitStatus, statusByRepo, displayGitStatus, displayStatusByRepo, displayScopeByRepo };
}

function displayRefreshState(
  status: GitStatusEntry,
  repositoryRefresh: GitStatusRefreshState | undefined,
  environmentRefresh: GitStatusRefreshState | undefined,
): "pending" | "unavailable" | undefined {
  let refresh = repositoryRefresh ?? environmentRefresh;
  if (repositoryRefresh?.request_id) refresh = repositoryRefresh;
  else if (environmentRefresh?.request_id) refresh = environmentRefresh;
  if (!refresh) return undefined;
  if (refresh.request_id) return refresh.state;

  const ordering = [
    [refresh.tracker_id, status.tracker_id],
    [refresh.tracker_epoch, status.tracker_epoch],
    [refresh.snapshot_revision, status.snapshot_revision],
    [refresh.timestamp, status.timestamp],
  ] as const;
  const hasOrdering = ordering.some(([incoming]) => incoming !== undefined);
  const matchesCurrentStatus =
    hasOrdering &&
    ordering.every(([incoming, current]) => incoming === undefined || incoming === current);
  return matchesCurrentStatus ? undefined : refresh.state;
}

export function useSessionGitStatusRefresh(sessionId: string | null): {
  environmentId: string | null;
  environment: GitStatusRefreshState | undefined;
  repositories: Record<string, GitStatusRefreshState> | undefined;
} {
  return useAppStore(
    useShallow((state) => {
      const environmentId = sessionId
        ? (state.environmentIdBySessionId[sessionId] ?? sessionId)
        : null;
      return {
        environmentId,
        environment: environmentId
          ? state.gitStatus.refreshByEnvironmentId?.[environmentId]
          : undefined,
        repositories: environmentId
          ? state.gitStatus.refreshByEnvironmentRepo?.[environmentId]
          : undefined,
      };
    }),
  );
}

/** Identifies the active pending-operation scope across session/environment replacement. */
export function useSessionGitPendingScope(sessionId: string | null): string {
  return useAppStore((state) => {
    if (!sessionId) return "";
    const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return `${sessionId}\u0000${envKey}`;
  });
}

/**
 * Checkout/reset generations are repository-scoped. Commit refetches and
 * comparison-base refreshes intentionally do not participate here because
 * they do not replace the checked-out worktree.
 */
export function useSessionGitPendingCheckoutGenerations(
  sessionId: string | null,
): Record<string, number> {
  return useAppStore(
    useShallow((state) => {
      if (!sessionId) return {};
      const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
      return state.gitCheckoutGeneration.byEnvironmentId[envKey] ?? {};
    }),
  );
}
