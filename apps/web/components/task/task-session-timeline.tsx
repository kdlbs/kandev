'use client';

import { useMemo, useState, useCallback } from 'react';
import {
  IconGitCommit,
  IconCamera,
  IconChevronDown,
  IconChevronRight,
  IconPlus,
  IconCircleFilled,
  IconMinus,
  IconLoader2,
} from '@tabler/icons-react';

import { cn } from '@/lib/utils';
import { useSessionGitSnapshots } from '@/hooks/domains/session/use-session-git-snapshots';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { GitSnapshot, SessionCommit, SnapshotType, FileInfo } from '@/lib/state/slices/session-runtime/types';

// Type for commit diff response
type CommitDiffResponse = {
  success: boolean;
  commit_sha: string;
  message: string;
  author: string;
  date: string;
  files: Record<string, FileInfo>;
  files_changed: number;
  insertions: number;
  deletions: number;
  error?: string;
};

interface TaskSessionTimelineProps {
  sessionId: string | null;
  onSelectDiff?: (path: string, content?: string) => void;
}

type TimelineItem =
  | { type: 'commit'; data: SessionCommit; timestamp: string }
  | { type: 'snapshot'; data: GitSnapshot; timestamp: string };

const snapshotTypeLabels: Record<SnapshotType, string> = {
  status_update: 'Status Update',
  pre_commit: 'Pre-Commit',
  post_commit: 'Post-Commit',
  pre_stage: 'Pre-Stage',
  post_stage: 'Post-Stage',
};

function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

const FileStatusIcon = ({ status }: { status: FileInfo['status'] }) => {
  switch (status) {
    case 'added':
    case 'untracked':
      return <IconPlus className="h-3 w-3 text-emerald-600" />;
    case 'modified':
      return <IconCircleFilled className="h-2 w-2 text-yellow-600" />;
    case 'deleted':
      return <IconMinus className="h-3 w-3 text-rose-600" />;
    default:
      return <IconCircleFilled className="h-2 w-2 text-muted-foreground" />;
  }
};

function FileList({ files, onFileClick }: { files: Record<string, FileInfo>; onFileClick: (path: string, file: FileInfo) => void }) {
  const fileEntries = Object.entries(files);
  if (fileEntries.length === 0) return null;

  return (
    <ul className="mt-2 pl-6 space-y-1">
      {fileEntries.map(([path, file]) => {
        const hasDiff = Boolean(file.diff);
        return (
          <li
            key={path}
            className={cn(
              'flex items-center gap-2 text-xs text-muted-foreground rounded px-1 -mx-1',
              hasDiff && 'hover:bg-muted/50 cursor-pointer'
            )}
            onClick={() => hasDiff && onFileClick(path, file)}
            title={hasDiff ? 'Click to view diff' : path}
          >
            <FileStatusIcon status={file.status} />
            <span className="truncate flex-1">
              {path}
            </span>
            {((file.additions ?? 0) > 0 || (file.deletions ?? 0) > 0) && (
              <span className="shrink-0">
                <span className="text-emerald-500">+{file.additions ?? 0}</span>
                {' / '}
                <span className="text-rose-500">-{file.deletions ?? 0}</span>
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function CommitItem({
  commit,
  sessionId,
  onSelectDiff
}: {
  commit: SessionCommit;
  sessionId: string;
  onSelectDiff?: (path: string, content?: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [files, setFiles] = useState<Record<string, FileInfo> | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchCommitDiff = useCallback(async () => {
    const client = getWebSocketClient();
    if (!client) return;

    setLoading(true);
    setError(null);
    try {
      const response = await client.request<CommitDiffResponse>(
        'session.commit_diff',
        { session_id: sessionId, commit_sha: commit.commit_sha },
        10000 // 10s timeout for potentially large diffs
      );

      if (response?.success && response.files) {
        setFiles(response.files);
      } else {
        setError(response?.error || 'Failed to load commit diff');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load commit diff');
    } finally {
      setLoading(false);
    }
  }, [sessionId, commit.commit_sha]);

  const handleToggle = () => {
    const willExpand = !expanded;
    setExpanded(willExpand);
    // Fetch diff on first expansion
    if (willExpand && files === null && !loading) {
      fetchCommitDiff();
    }
  };

  const handleFileClick = (path: string, file: FileInfo) => {
    if (onSelectDiff && file.diff) {
      onSelectDiff(path, file.diff);
    }
  };

  return (
    <div className="flex flex-col gap-1">
      <button
        type="button"
        className="flex items-center gap-2 text-left hover:bg-muted/50 -mx-1 px-1 rounded"
        onClick={handleToggle}
      >
        {expanded ? (
          <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
        ) : (
          <IconChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />
        )}
        <IconGitCommit className="h-4 w-4 text-emerald-500 shrink-0" />
        <span className="font-medium text-sm truncate">Commit</span>
      </button>
      <p className="text-sm text-foreground/80 line-clamp-2 pl-6">
        {commit.commit_message}
      </p>
      <div className="flex items-center gap-2 pl-6 text-xs text-muted-foreground">
        <span>{commit.files_changed} file{commit.files_changed !== 1 ? 's' : ''}</span>
        <span className="text-emerald-500">+{commit.insertions}</span>
        <span className="text-rose-500">-{commit.deletions}</span>
      </div>
      {expanded && (
        <div className="pl-6">
          {loading && (
            <div className="flex items-center gap-2 text-xs text-muted-foreground py-2">
              <IconLoader2 className="h-3 w-3 animate-spin" />
              <span>Loading diff...</span>
            </div>
          )}
          {error && (
            <div className="text-xs text-destructive py-2">
              {error}
            </div>
          )}
          {files && Object.keys(files).length > 0 && (
            <FileList files={files} onFileClick={handleFileClick} />
          )}
          {files && Object.keys(files).length === 0 && (
            <div className="text-xs text-muted-foreground py-2">
              No file changes in this commit
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function SnapshotItem({ snapshot, onSelectDiff }: { snapshot: GitSnapshot; onSelectDiff?: (path: string, content?: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const files = snapshot.files ?? {};
  const filesCount = Object.keys(files).length;

  const handleFileClick = (path: string, file: FileInfo) => {
    if (onSelectDiff && file.diff) {
      onSelectDiff(path, file.diff);
    }
  };

  return (
    <div className="flex flex-col">
      <button
        type="button"
        className="flex items-center gap-1.5 text-left hover:bg-muted/50 -mx-1 px-1 py-0.5 rounded"
        onClick={() => filesCount > 0 && setExpanded(!expanded)}
        disabled={filesCount === 0}
      >
        {filesCount > 0 ? (
          expanded ? (
            <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
          ) : (
            <IconChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />
          )
        ) : (
          <span className="w-3" />
        )}
        <IconCamera className="h-3.5 w-3.5 text-blue-500 shrink-0" />
        <span className="text-xs text-muted-foreground">
          {filesCount > 0 ? (
            <>{filesCount} file{filesCount !== 1 ? 's' : ''} changed</>
          ) : (
            'No changes'
          )}
        </span>
      </button>
      {expanded && filesCount > 0 && <FileList files={files} onFileClick={handleFileClick} />}
    </div>
  );
}

export function TaskSessionTimeline({ sessionId, onSelectDiff }: TaskSessionTimelineProps) {
  const { snapshots, loading: snapshotsLoading } = useSessionGitSnapshots(sessionId);
  const { commits, loading: commitsLoading } = useSessionCommits(sessionId);

  const timeline = useMemo<TimelineItem[]>(() => {
    const items: TimelineItem[] = [
      ...snapshots.map((s) => ({
        type: 'snapshot' as const,
        data: s,
        timestamp: s.created_at,
      })),
      ...commits.map((c) => ({
        type: 'commit' as const,
        data: c,
        timestamp: c.committed_at,
      })),
    ];
    return items.sort(
      (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
    );
  }, [snapshots, commits]);

  const isLoading = snapshotsLoading || commitsLoading;

  if (!sessionId) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        No session selected
      </div>
    );
  }

  if (isLoading && timeline.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        Loading timeline...
      </div>
    );
  }

  if (timeline.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        No activity yet
      </div>
    );
  }

  return (
    <div className="space-y-0">
      {timeline.map((item, idx) => (
        <div
          key={`${item.type}-${item.type === 'commit' ? item.data.id : item.data.id}`}
          className={cn(
            'relative border-l-2 border-border pl-4 pb-4 ml-2',
            idx === timeline.length - 1 && 'border-transparent'
          )}
        >
          <div
            className={cn(
              'absolute -left-[5px] top-0 h-2 w-2 rounded-full',
              item.type === 'commit' ? 'bg-emerald-500' : 'bg-blue-500'
            )}
          />
          {item.type === 'commit' ? (
            <CommitItem commit={item.data} sessionId={sessionId} onSelectDiff={onSelectDiff} />
          ) : (
            <SnapshotItem snapshot={item.data} onSelectDiff={onSelectDiff} />
          )}
          <div className="text-xs text-muted-foreground mt-1 pl-6">
            {formatRelativeTime(item.timestamp)}
          </div>
        </div>
      ))}
    </div>
  );
}

