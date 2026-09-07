"use client";

import { useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { PRCommitInfo, TaskPR } from "@/lib/types/github";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import { usePRReviewRepositoryIdentityResolver } from "@/hooks/domains/github/use-pr-review-repository-identity";
import { useReviewPRSelection } from "@/hooks/domains/github/use-review-pr-selection";
import { useSessionGitStatusByRepo } from "./use-session-git-status";
import { resolveBranchScopedTaskPRs, selectBranchScopedTaskPR } from "./branch-scoped-task-pr";
import {
  classifyRemoteContribution,
  type RemoteContributionRelation,
} from "./remote-contribution-relation";

export type RemoteContributionRelationState = {
  prs: TaskPR[];
  selectedPR: TaskPR | null;
  repositoryName: string | undefined;
  commits: PRCommitInfo[];
  loading: boolean;
  error: string | null;
  relation: RemoteContributionRelation;
  refreshProviderEvidence: () => Promise<string | null>;
};

export function useRemoteContributionRelation(
  sessionId: string | null | undefined,
): RemoteContributionRelationState {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const { prs, selectedKey } = useReviewPRSelection(activeTaskId);
  const statusByRepo = useSessionGitStatusByRepo(sessionId ?? null);
  const resolveRepositoryName = usePRReviewRepositoryIdentityResolver(activeTaskId, sessionId);
  const branchScopedPRs = useMemo(
    () =>
      resolveBranchScopedTaskPRs({
        prs,
        statuses: statusByRepo,
        preferredKey: selectedKey,
        resolveRepositoryName,
      }),
    [prs, statusByRepo, selectedKey, resolveRepositoryName],
  );
  const selection = useMemo(
    () => selectBranchScopedTaskPR(branchScopedPRs, selectedKey),
    [branchScopedPRs, selectedKey],
  );
  const scopedPRs = useMemo(() => branchScopedPRs.map((entry) => entry.pr), [branchScopedPRs]);
  const selectedPR = selection?.pr ?? null;
  const commitsState = usePRCommits(
    selectedPR?.owner ?? null,
    selectedPR?.repo ?? null,
    selectedPR?.pr_number ?? null,
    selectedPR?.last_synced_at ?? null,
  );
  const repositoryName = selection?.repositoryName;
  const refreshProviderEvidence = useCallback(async () => {
    const refreshed = await commitsState.refresh();
    return refreshed?.providerHead ?? null;
  }, [commitsState.refresh]);
  const gitStatus = selection?.gitStatus;

  const relation = useMemo(
    () =>
      classifyRemoteContribution({
        hasSelectedPR: Boolean(selectedPR),
        providerCommits: commitsState.authoritativeCommits,
        providerHead: commitsState.providerHead,
        providerCommitsComplete: commitsState.providerCommitsComplete,
        providerLoading: commitsState.loading,
        providerError: commitsState.error,
        localHead: gitStatus?.head_commit,
        upstreamHead: gitStatus?.remote_head_commit,
        remoteAhead: gitStatus?.remote_ahead ?? 0,
        remoteBehind: gitStatus?.remote_behind ?? 0,
        baseAhead: gitStatus?.ahead ?? 0,
        hasUpstream: Boolean(gitStatus?.remote_branch),
      }),
    [
      selectedPR,
      commitsState.authoritativeCommits,
      commitsState.providerHead,
      commitsState.providerCommitsComplete,
      commitsState.loading,
      commitsState.error,
      gitStatus,
    ],
  );

  return {
    prs: scopedPRs,
    selectedPR,
    repositoryName,
    commits: commitsState.commits,
    loading: commitsState.loading,
    error: commitsState.error,
    relation,
    refreshProviderEvidence,
  };
}
