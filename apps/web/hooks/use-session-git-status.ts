import { useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { GitStatusEntry } from '@/lib/state/store';

export function useSessionGitStatus(sessionId: string | null) {
  const gitStatus = useAppStore((state) =>
    sessionId ? state.gitStatus.bySessionId[sessionId] : undefined
  );
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined
  );
  const setGitStatus = useAppStore((state) => state.setGitStatus);

  // Populate git status from session metadata if not already in store
  useEffect(() => {
    if (!sessionId || gitStatus) return;

    // Try to extract git_status from session metadata
    const metadata = session?.metadata;
    if (!metadata || typeof metadata !== 'object') return;

    const storedGitStatus = metadata.git_status;
    if (!storedGitStatus || typeof storedGitStatus !== 'object') return;

    // Map stored git status to GitStatusEntry
    const gs = storedGitStatus as Record<string, unknown>;
    const entry: GitStatusEntry = {
      branch: (gs.branch as string) ?? null,
      remote_branch: (gs.remote_branch as string) ?? null,
      modified: (gs.modified as string[]) ?? [],
      added: (gs.added as string[]) ?? [],
      deleted: (gs.deleted as string[]) ?? [],
      untracked: (gs.untracked as string[]) ?? [],
      renamed: (gs.renamed as string[]) ?? [],
      ahead: (gs.ahead as number) ?? 0,
      behind: (gs.behind as number) ?? 0,
      files: (gs.files as Record<string, GitStatusEntry['files'][string]>) ?? {},
      timestamp: (gs.timestamp as string) ?? null,
    };

    setGitStatus(sessionId, entry);
  }, [sessionId, gitStatus, session?.metadata, setGitStatus]);

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
