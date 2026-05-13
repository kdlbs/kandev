"use client";

import { memo, useEffect, useMemo } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { FileDiffViewer } from "@/components/diff";
import { useAppStore } from "@/components/state-provider";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useCommitDiff } from "@/hooks/domains/session/use-commit-diff";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import type { FileInfo } from "@/lib/state/store";

type CommitDetailPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

type CommitDiffViewProps = {
  sha: string;
  repo?: string;
  onOpenFile?: (path: string) => void;
  wordWrap?: boolean;
};

function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin} minute${diffMin !== 1 ? "s" : ""} ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? "s" : ""} ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 30) return `${diffDays} day${diffDays !== 1 ? "s" : ""} ago`;
  return date.toLocaleDateString();
}

function getInitials(name: string): string {
  return name
    .split(/\s+/)
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
}

function useSortedFileEntries(files: Record<string, FileInfo> | null): [string, FileInfo][] {
  return useMemo(() => {
    if (!files) return [];
    return Object.entries(files).sort(([a], [b]) => a.localeCompare(b));
  }, [files]);
}

function useActiveCommit(commitSha: string) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  return useMemo(() => commits.find((c) => c.commit_sha === commitSha), [commits, commitSha]);
}

/** Standalone commit diff viewer — no dockview dependencies. */
export const CommitDiffView = memo(function CommitDiffView({
  sha: commitSha,
  repo,
  onOpenFile,
  wordWrap,
}: CommitDiffViewProps) {
  const commit = useActiveCommit(commitSha);
  const { files, loading } = useCommitDiff(commitSha, repo);
  const fileEntries = useSortedFileEntries(files);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full gap-2 text-muted-foreground text-sm">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        Loading commit...
      </div>
    );
  }

  return (
    <div className="overflow-y-auto">
      <div className="p-3">{commit && <CommitHeader commit={commit} commitSha={commitSha} />}</div>
      <CommitFileList
        fileEntries={fileEntries}
        loading={loading}
        onOpenFile={onOpenFile ?? (() => {})}
        baseRef={`${commitSha}^`}
        repo={repo}
        wordWrap={wordWrap}
      />
    </div>
  );
});

const CommitDetailPanel = memo(function CommitDetailPanel({
  panelId,
  params,
}: CommitDetailPanelProps) {
  const commitSha = params.commitSha as string;
  const repo = (params.repo as string | undefined) ?? undefined;
  const { openFile } = usePanelActions();
  const commit = useActiveCommit(commitSha);
  const { files, loading } = useCommitDiff(commitSha, repo);
  const fileEntries = useSortedFileEntries(files);

  // Update tab title via dockview API stored in portal manager
  useEffect(() => {
    if (commit) {
      const shortSha = commitSha.slice(0, 7);
      const msg =
        commit.commit_message.length > 30
          ? commit.commit_message.slice(0, 30) + "..."
          : commit.commit_message;
      setPanelTitle(panelId, `${shortSha} ${msg}`);
    }
  }, [commit, commitSha, panelId]);

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
        <CommitFileList
          fileEntries={fileEntries}
          loading={loading}
          onOpenFile={openFile}
          baseRef={`${commitSha}^`}
          repo={repo}
        />
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
  baseRef,
  repo,
  wordWrap,
}: {
  fileEntries: [string, FileInfo][];
  loading: boolean;
  onOpenFile: (path: string) => void;
  baseRef: string;
  repo?: string;
  wordWrap?: boolean;
}) {
  if (fileEntries.length === 0 && !loading) {
    return (
      <div className="text-sm text-muted-foreground text-center py-8">No files in this commit</div>
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
              enableExpansion={true}
              baseRef={baseRef}
              repo={repo}
              wordWrap={wordWrap}
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
