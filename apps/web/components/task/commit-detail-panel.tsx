"use client";

import { memo, useEffect, useMemo } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { FileDiffViewer } from "@/components/diff";
import { DEFAULT_DIFF_WORD_WRAP } from "@/components/diff/diff-defaults";
import { useAppStore } from "@/components/state-provider";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useCommitDetail } from "@/hooks/domains/session/use-commit-detail";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { formatRelativeTime } from "@/lib/utils";
import type { FileInfo } from "@/lib/state/store";
import type { CommitDetailTarget } from "./changes-diff-target";

type CommitDetailPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

type CommitDiffViewProps = {
  target: CommitDetailTarget;
  onOpenFile?: (path: string) => void;
  wordWrap?: boolean;
};

function isCommitDetailTarget(value: unknown): value is CommitDetailTarget {
  if (!value || typeof value !== "object") return false;
  const target = value as { source?: unknown; sha?: unknown };
  return (
    (target.source === "local" || target.source === "github") && typeof target.sha === "string"
  );
}

function targetFromParams(params: Record<string, unknown>): CommitDetailTarget {
  if (isCommitDetailTarget(params.target)) return params.target;
  return {
    source: "local",
    sha: typeof params.commitSha === "string" ? params.commitSha : "",
    ...(typeof params.repo === "string" && params.repo ? { repo: params.repo } : {}),
  };
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

function useActiveCommit(target: CommitDetailTarget) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  return useMemo(() => {
    if (target.source !== "local") return undefined;
    return commits.find(
      (c) =>
        c.commit_sha === target.sha &&
        (!target.repo || !c.repository_name || c.repository_name === target.repo),
    );
  }, [commits, target]);
}

function headerCommit(
  target: CommitDetailTarget,
  localCommit: ReturnType<typeof useActiveCommit>,
  remoteCommit: ReturnType<typeof useCommitDetail>["commit"],
) {
  if (target.source === "github" && remoteCommit) {
    return {
      author_name: remoteCommit.author_name || remoteCommit.author_login,
      commit_message: remoteCommit.message,
      committed_at: remoteCommit.author_date,
    };
  }
  return localCommit;
}

/** Standalone commit diff viewer — no dockview dependencies. */
export const CommitDiffView = memo(function CommitDiffView({
  target,
  onOpenFile,
  wordWrap = DEFAULT_DIFF_WORD_WRAP,
}: CommitDiffViewProps) {
  const localCommit = useActiveCommit(target);
  const { files, commit: remoteCommit, loading } = useCommitDetail(target);
  const commit = headerCommit(target, localCommit, remoteCommit);
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
      <div className="p-3">{commit && <CommitHeader commit={commit} commitSha={target.sha} />}</div>
      <CommitFileList
        fileEntries={fileEntries}
        loading={loading}
        onOpenFile={target.source === "local" ? onOpenFile : undefined}
        baseRef={target.source === "local" ? `${target.sha}^` : undefined}
        repo={target.source === "local" ? target.repo : undefined}
        remote={target.source === "github"}
        wordWrap={wordWrap}
      />
    </div>
  );
});

const CommitDetailPanel = memo(function CommitDetailPanel({
  panelId,
  params,
}: CommitDetailPanelProps) {
  const target = targetFromParams(params);
  const { openFile } = usePanelActions();
  const localCommit = useActiveCommit(target);
  const { files, commit: remoteCommit, loading } = useCommitDetail(target);
  const commit = headerCommit(target, localCommit, remoteCommit);
  const fileEntries = useSortedFileEntries(files);

  // Update tab title via dockview API stored in portal manager
  useEffect(() => {
    if (commit) {
      const shortSha = target.sha.slice(0, 7);
      const msg =
        commit.commit_message.length > 30
          ? commit.commit_message.slice(0, 30) + "..."
          : commit.commit_message;
      setPanelTitle(panelId, `${shortSha} ${msg}`);
    }
  }, [commit, panelId, target.sha]);

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
          {commit && <CommitHeader commit={commit} commitSha={target.sha} />}
        </div>
        <CommitFileList
          fileEntries={fileEntries}
          loading={loading}
          onOpenFile={target.source === "local" ? openFile : undefined}
          baseRef={target.source === "local" ? `${target.sha}^` : undefined}
          repo={target.source === "local" ? target.repo : undefined}
          remote={target.source === "github"}
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
  remote,
  wordWrap,
}: {
  fileEntries: [string, FileInfo][];
  loading: boolean;
  onOpenFile?: (path: string) => void;
  baseRef?: string;
  repo?: string;
  remote: boolean;
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
              enableExpansion={!remote}
              baseRef={baseRef}
              repo={remote ? undefined : repo}
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
