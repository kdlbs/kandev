import { useEffect, useCallback } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { GitSnapshot } from '@/lib/state/slices/session-runtime/types';

/**
 * Hook to get the current git status for a session.
 * Git status is populated via WebSocket from git snapshot updates.
 * On mount, fetches the latest snapshot to populate initial status.
 * For historical snapshots, use useSessionGitSnapshots hook.
 */
export function useSessionGitStatus(sessionId: string | null) {
  const gitStatus = useAppStore((state) =>
    sessionId ? state.gitStatus.bySessionId[sessionId] : undefined
  );
  const setGitStatus = useAppStore((state) => state.setGitStatus);

  // Fetch latest git snapshot to populate initial status
  const fetchLatestSnapshot = useCallback(async () => {
    if (!sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    try {
      const response = await client.request<{ snapshots?: GitSnapshot[] }>(
        'session.git.snapshots',
        {
          session_id: sessionId,
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
        setGitStatus(sessionId, {
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
  }, [sessionId, setGitStatus]);

  // Fetch initial status on mount if not already loaded
  useEffect(() => {
    if (sessionId && !gitStatus) {
      fetchLatestSnapshot();
    }
  }, [sessionId, gitStatus, fetchLatestSnapshot]);

  // Subscribe to session updates to receive git status via WebSocket
  useEffect(() => {
    if (!sessionId) return;
    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        // Don't clear git status on cleanup - keep it cached for when user switches back
      };
    }
  }, [sessionId]);

  return gitStatus;
}
