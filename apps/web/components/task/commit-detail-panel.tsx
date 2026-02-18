'use client';

import { memo, useState, useEffect, useCallback, useMemo } from 'react';
import type { IDockviewPanelProps } from 'dockview-react';
import { IconLoader2 } from '@tabler/icons-react';
import { PanelRoot, PanelBody } from './panel-primitives';
import { FileDiffViewer } from '@/components/diff';
import { useAppStore } from '@/components/state-provider';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useToast } from '@/components/toast-provider';
import { usePanelActions } from '@/hooks/use-panel-actions';
import type { FileInfo } from '@/lib/state/store';

type CommitDetailParams = {
  commitSha: string;
};

function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return 'just now';
  if (diffMin < 60) return `${diffMin} minute${diffMin !== 1 ? 's' : ''} ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 30) return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
  return date.toLocaleDateString();
}

function getInitials(name: string): string {
  return name
    .split(/\s+/)
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join('')
    .toUpperCase();
}

const CommitDetailPanel = memo(function CommitDetailPanel(
  props: IDockviewPanelProps<CommitDetailParams>,
) {
  const commitSha = props.params.commitSha;
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const { toast } = useToast();
  const { openFile } = usePanelActions();

  const [files, setFiles] = useState<Record<string, FileInfo> | null>(null);
  const [loading, setLoading] = useState(false);

  // Find commit metadata
  const commit = useMemo(
    () => commits.find((c) => c.commit_sha === commitSha),
    [commits, commitSha],
  );

  // Update tab title
  useEffect(() => {
    if (commit) {
      const shortSha = commitSha.slice(0, 7);
      const msg = commit.commit_message.length > 30
        ? commit.commit_message.slice(0, 30) + '...'
        : commit.commit_message;
      props.api.setTitle(`${shortSha} ${msg}`);
    }
  }, [commit, commitSha, props.api]);

  // Fetch diff
  const fetchDiff = useCallback(async () => {
    if (!activeSessionId) return;
    setLoading(true);
    try {
      const ws = getWebSocketClient();
      if (!ws) return;
      const response = await ws.request<{ success: boolean; files: Record<string, FileInfo> }>(
        'session.commit_diff',
        { session_id: activeSessionId, commit_sha: commitSha },
        10000,
      );
      if (response?.success && response.files) {
        setFiles(response.files);
      }
    } catch (err) {
      toast({
        title: 'Failed to load commit diff',
        description: err instanceof Error ? err.message : 'An unexpected error occurred',
        variant: 'error',
      });
    } finally {
      setLoading(false);
    }
  }, [activeSessionId, commitSha, toast]);

  useEffect(() => {
    fetchDiff();
  }, [fetchDiff]);

  const fileEntries = useMemo(() => {
    if (!files) return [];
    return Object.entries(files).sort(([a], [b]) => a.localeCompare(b));
  }, [files]);

  if (loading) {
    return (
      <PanelRoot>
        <PanelBody>
          <div className="flex items-center justify-center h-full gap-2 text-muted-foreground text-sm">
            <IconLoader2 className="h-4 w-4 animate-spin" />
            Loading commit...
          </div>
        </PanelBody>
      </PanelRoot>
    );
  }

  return (
    <PanelRoot>
      <PanelBody padding={false} scroll>
        <div className="p-3">
          {commit && <CommitHeader commit={commit} commitSha={commitSha} />}
        </div>
        <CommitFileList fileEntries={fileEntries} loading={loading} onOpenFile={openFile} />
      </PanelBody>
    </PanelRoot>
  );
});

/** Commit metadata header with author and message */
function CommitHeader({
  commit,
  commitSha,
}: {
  commit: { author_name: string; commit_message: string; committed_at: string };
  commitSha: string;
}) {
  return (
    <div className="mb-4 pb-3 border-b border-border">
      <div className="flex items-start gap-3">
        <div className="flex items-center justify-center size-8 rounded-full bg-muted text-xs font-semibold text-muted-foreground shrink-0">
          {getInitials(commit.author_name)}
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-foreground leading-snug">
            {commit.commit_message}
          </p>
          <p className="text-xs text-muted-foreground mt-1">
            {commit.author_name}
            <span className="mx-1.5">&middot;</span>
            {formatRelativeTime(commit.committed_at)}
            <span className="mx-1.5">&middot;</span>
            <code className="font-mono text-[11px]">{commitSha.slice(0, 7)}</code>
          </p>
        </div>
      </div>
    </div>
  );
}

/** List of file diffs in a commit */
function CommitFileList({
  fileEntries,
  loading,
  onOpenFile,
}: {
  fileEntries: [string, FileInfo][];
  loading: boolean;
  onOpenFile: (path: string) => void;
}) {
  if (fileEntries.length === 0 && !loading) {
    return (
      <div className="text-sm text-muted-foreground text-center py-8">
        No files in this commit
      </div>
    );
  }
  return (
    <>
      {fileEntries.map(([path, file]) => (
        <div key={path} className="mb-2">
          {file.diff ? (
            <FileDiffViewer
              filePath={path}
              diff={file.diff}
              status={file.status}
              onOpenFile={onOpenFile}
            />
          ) : (
            <div className="px-3 py-2 text-xs text-muted-foreground">
              {path} -- binary or empty diff
            </div>
          )}
        </div>
      ))}
    </>
  );
}

export { CommitDetailPanel };
