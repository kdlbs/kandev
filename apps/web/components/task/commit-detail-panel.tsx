"use client";

import { memo, useEffect, useMemo } from "react";
import { IconAlertCircle, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { DEFAULT_DIFF_WORD_WRAP } from "@/components/diff/diff-defaults";
import { useAppStore } from "@/components/state-provider";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useCommitDetail } from "@/hooks/domains/session/use-commit-detail";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import type { CommitDetailTarget, CommitFileNavigationRequest } from "./changes-diff-target";
import { CommitDetailContent, type CommitDetailHeaderCommit } from "./commit-detail-content";

type CommitDetailPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

type CommitDiffViewProps = {
  target: CommitDetailTarget;
  onOpenFile?: (path: string, repo?: string) => void;
  wordWrap?: boolean;
  fileNavigation?: CommitFileNavigationRequest | null;
};

function isCommitDetailTarget(value: unknown): value is CommitDetailTarget {
  if (!value || typeof value !== "object") return false;
  const target = value as {
    source?: unknown;
    sha?: unknown;
    repo?: unknown;
    workspaceId?: unknown;
    owner?: unknown;
  };
  if (typeof target.sha !== "string" || target.sha.length === 0) return false;
  if (target.source === "local") {
    return target.repo === undefined || typeof target.repo === "string";
  }
  return (
    target.source === "github" &&
    typeof target.workspaceId === "string" &&
    target.workspaceId.length > 0 &&
    typeof target.owner === "string" &&
    target.owner.length > 0 &&
    typeof target.repo === "string" &&
    target.repo.length > 0
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

function fileNavigationFromParams(
  params: Record<string, unknown>,
): CommitFileNavigationRequest | undefined {
  const value = params.fileNavigation;
  if (!value || typeof value !== "object") return undefined;
  const navigation = value as { path?: unknown; token?: unknown };
  if (
    typeof navigation.path !== "string" ||
    navigation.path.length === 0 ||
    typeof navigation.token !== "number" ||
    !Number.isFinite(navigation.token)
  ) {
    return undefined;
  }
  return { path: navigation.path, token: navigation.token };
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
): CommitDetailHeaderCommit | undefined {
  if (target.source === "github" && remoteCommit) {
    return {
      authorName: remoteCommit.author_name || remoteCommit.author_login,
      commitMessage: remoteCommit.message,
      committedAt: remoteCommit.author_date,
      repositoryName: target.repositoryName ?? target.repo,
    };
  }
  if (!localCommit) return undefined;
  return {
    authorName: localCommit.author_name,
    commitMessage: localCommit.commit_message,
    committedAt: localCommit.committed_at,
    repositoryName: localCommit.repository_name,
  };
}

function commitTargetKey(target: CommitDetailTarget, sessionId?: string | null): string {
  const sessionKey = sessionId ?? "";
  if (target.source === "local") return `local:${sessionKey}:${target.repo ?? ""}:${target.sha}`;
  return `github:${sessionKey}:${target.workspaceId}:${target.owner}/${target.repo}:${target.sha}`;
}

/** Standalone commit diff viewer — no dockview dependencies. */
export const CommitDiffView = memo(function CommitDiffView({
  target,
  onOpenFile,
  wordWrap = DEFAULT_DIFF_WORD_WRAP,
  fileNavigation,
}: CommitDiffViewProps) {
  const { t } = useTranslation();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const localCommit = useActiveCommit(target);
  const { files, commit: remoteCommit, loading, error, refetch } = useCommitDetail(target);
  const commit = headerCommit(target, localCommit, remoteCommit);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full gap-2 text-muted-foreground text-sm">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        {t("task:loadingCommit")}
      </div>
    );
  }
  if (error) return <CommitDetailErrorState error={error} onRetry={refetch} />;

  return (
    <CommitDetailContent
      key={commitTargetKey(target, activeSessionId)}
      target={target}
      fileEntries={files ? Object.entries(files) : []}
      commit={commit}
      sessionId={activeSessionId}
      initialWordWrap={wordWrap}
      onOpenFile={target.source === "local" ? onOpenFile : undefined}
      fileNavigation={fileNavigation}
    />
  );
});

const CommitDetailPanel = memo(function CommitDetailPanel({
  panelId,
  params,
}: CommitDetailPanelProps) {
  const { t } = useTranslation();
  const target = targetFromParams(params);
  const fileNavigation = fileNavigationFromParams(params);
  const { openFile } = usePanelActions();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const localCommit = useActiveCommit(target);
  const { files, commit: remoteCommit, loading, error, refetch } = useCommitDetail(target);
  const commit = headerCommit(target, localCommit, remoteCommit);

  // Update tab title via dockview API stored in portal manager
  useEffect(() => {
    if (commit) {
      const shortSha = target.sha.slice(0, 7);
      const msg =
        commit.commitMessage.length > 30
          ? commit.commitMessage.slice(0, 30) + "..."
          : commit.commitMessage;
      setPanelTitle(panelId, `${shortSha} ${msg}`);
    }
  }, [commit, panelId, target.sha]);

  if (loading) {
    return (
      <PanelRoot>
        <PanelBody>
          <div className="flex items-center justify-center h-full gap-2 text-muted-foreground text-sm">
            <IconLoader2 className="h-4 w-4 animate-spin" />
            {t("task:loadingCommit")}
          </div>
        </PanelBody>
      </PanelRoot>
    );
  }
  if (error) {
    return (
      <PanelRoot>
        <PanelBody>
          <CommitDetailErrorState error={error} onRetry={refetch} />
        </PanelBody>
      </PanelRoot>
    );
  }

  return (
    <PanelRoot>
      <PanelBody padding={false} scroll>
        <CommitDetailContent
          key={commitTargetKey(target, activeSessionId)}
          target={target}
          fileEntries={files ? Object.entries(files) : []}
          commit={commit}
          sessionId={activeSessionId}
          onOpenFile={target.source === "local" ? openFile : undefined}
          fileNavigation={fileNavigation}
        />
      </PanelBody>
    </PanelRoot>
  );
});

function CommitDetailErrorState({
  error,
  onRetry,
}: {
  error: string;
  onRetry: () => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <div
      role="alert"
      className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center text-sm text-muted-foreground"
    >
      <IconAlertCircle className="h-5 w-5 text-destructive" />
      <span>{error}</span>
      <Button
        type="button"
        className="cursor-pointer"
        variant="outline"
        onClick={() => void onRetry()}
      >
        <IconRefresh className="h-4 w-4" />
        {t("system:featureTogglesRetry")}
      </Button>
    </div>
  );
}

export { CommitDetailPanel };
