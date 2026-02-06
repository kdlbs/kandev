import { useEffect, useRef, useCallback } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { GitSnapshot } from '@/lib/state/slices/session-runtime/types';

/**
 * Hook to get the current git status for a session.
 * Git status is populated via WebSocket from git snapshot updates.
 * On mount, fetches the latest snapshot to populate initial status.
 * For historical snapshots, use useSessionGitSnapshots hook.
 */
export function useSessionGitStatus(sessionId: string | null) {
  // Use shallow comparison to prevent re-renders when object reference changes but values are the same
  const gitStatus = useAppStore(
    useShallow((state) =>
      sessionId ? state.gitStatus.bySessionId[sessionId] : undefined
    )
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const storeApi = useAppStoreApi();
  const prevSessionIdRef = useRef<string | null>(null);
  const hasFetchedRef = useRef(false);

  // Stable reference to fetch function
  const fetchSnapshot = useCallback(async (sid: string) => {
    const client = getWebSocketClient();
    if (!client) return;

    const setGitStatus = storeApi.getState().setGitStatus;

    try {
      const response = await client.request<{ snapshots?: GitSnapshot[] }>(
        'session.git.snapshots',
        {
          session_id: sid,
          limit: 1, // Only fetch the latest snapshot
        }
      );

      if (response?.snapshots && response.snapshots.length > 0) {
        const latest = response.snapshots[0];

        // Extract file paths by status from the files object
        const modified: string[] = [];
        const added: string[] = [];
        const deleted: string[] = [];
        const untracked: string[] = [];
        const renamed: string[] = [];

        if (latest.files) {
          Object.entries(latest.files).forEach(([path, fileInfo]) => {
            const status = fileInfo.status?.toLowerCase();
            if (status === 'modified') modified.push(path);
            else if (status === 'added') added.push(path);
            else if (status === 'deleted') deleted.push(path);
            else if (status === 'untracked') untracked.push(path);
            else if (status === 'renamed') renamed.push(path);
          });
        }

        // Populate git status from latest snapshot
        setGitStatus(sid, {
          branch: latest.branch,
          remote_branch: latest.remote_branch,
          modified,
          added,
          deleted,
          untracked,
          renamed,
          ahead: latest.ahead,
          behind: latest.behind,
          files: latest.files,
          timestamp: latest.created_at,
        });
      }
    } catch (error) {
      console.error('Failed to fetch latest git snapshot:', error);
    }
  }, [storeApi]);

  // Fetch initial status on mount if not already loaded
  // Also refetch when switching to a different session or when WebSocket connects
  useEffect(() => {
    if (!sessionId) return;

    // Wait for WebSocket to be connected before fetching
    if (connectionStatus !== 'connected') return;

    // Detect session change to force refetch
    const sessionChanged = prevSessionIdRef.current !== null &&
                           prevSessionIdRef.current !== sessionId;

    if (sessionChanged) {
      hasFetchedRef.current = false;
    }
    prevSessionIdRef.current = sessionId;

    // Skip fetch if we already have status or already fetched for this session
    const currentStatus = storeApi.getState().gitStatus.bySessionId[sessionId];
    if (currentStatus || hasFetchedRef.current) {
      return;
    }

    hasFetchedRef.current = true;
    fetchSnapshot(sessionId);
  }, [sessionId, connectionStatus, fetchSnapshot, storeApi]);

  // Subscribe to session updates to receive git status via WebSocket
  useEffect(() => {
    if (!sessionId) return;

    // Wait for WebSocket to be connected before subscribing
    if (connectionStatus !== 'connected') return;

    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        // Don't clear git status on cleanup - keep it cached for when user switches back
      };
    }
  }, [sessionId, connectionStatus]);

  return gitStatus;
}
