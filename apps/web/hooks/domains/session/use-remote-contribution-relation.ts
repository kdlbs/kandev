"use client";

import { useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { PRCommitInfo, TaskPR } from "@/lib/types/github";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import { usePRReviewRepositoryIdentityResolver } from "@/hooks/domains/github/use-pr-review-repository-identity";
import { useReviewPRSelection } from "@/hooks/domains/github/use-review-pr-selection";
import { useSessionGitStatusByRepo } from "./use-session-git-status";
import { resolveBranchScopedTaskPRs, selectBranchScopedTaskPR } from "./branch-scoped-task-pr";
import { prTaskKey } from "@/components/github/pr-utils";
import {
  classifyRemoteContribution,
  type RemoteContributionRelation,
} from "./remote-contribution-relation";
import type { ContributionHistoryExplanationTarget } from "./use-contribution-history-explanation";

export type RemoteContributionRelationState = {
  prs: TaskPR[];
  selectedPR: TaskPR | null;
  /** The repository key used by Git operations. Empty means the single-repo root. */
  repositoryScope: string;
  repositoryName: string | undefined;
  commits: PRCommitInfo[];
  loading: boolean;
  error: string | null;
  relation: RemoteContributionRelation;
  refreshProviderEvidence: () => Promise<string | null>;
  contributionHistoryTarget: ContributionHistoryExplanationTarget | null;
};

function buildContributionHistoryTarget(
  sessionId: string | null | undefined,
  selectedPR: TaskPR | null,
  gitStatus: ReturnType<typeof useSessionGitStatusByRepo>[number]["status"] | undefined,
  providerHead: string | null | undefined,
): ContributionHistoryExplanationTarget | null {
  if (!sessionId || !selectedPR || !gitStatus?.branch) return null;
  return {
    sessionId,
    workspaceId: selectedPR.workspace_id,
    repositoryScope: gitStatus.repository_name ?? "",
    branch: gitStatus.branch,
    selectedPRKey: prTaskKey(selectedPR),
    expectedLocalHead: gitStatus.head_commit ?? "",
    expectedRemoteHead: providerHead ?? "",
  };
}

function classifyContributionRelation(
  input: Parameters<typeof classifyRemoteContribution>[0],
): RemoteContributionRelation {
  return classifyRemoteContribution(input);
}

function useSelectedPRCommits(selectedPR: TaskPR | null) {
  return usePRCommits(
    selectedPR?.owner ?? null,
    selectedPR?.repo ?? null,
    selectedPR?.pr_number ?? null,
    selectedPR?.last_synced_at ?? null,
  );
}

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
  const commitsState = useSelectedPRCommits(selectedPR);
  const repositoryName = selection?.repositoryName;
  const repositoryScope = selection?.gitStatus.repository_name ?? "";
  const refreshProviderEvidence = useCallback(async () => {
    const refreshed = await commitsState.refresh();
    return refreshed?.providerHead ?? null;
  }, [commitsState.refresh]);
  const gitStatus = selection?.gitStatus;
  const contributionHistoryTarget = useMemo(
    () =>
      buildContributionHistoryTarget(sessionId, selectedPR, gitStatus, commitsState.providerHead),
    [sessionId, selectedPR, gitStatus, commitsState.providerHead],
  );

  const relation = useMemo(
    () =>
      classifyContributionRelation({
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
    repositoryScope,
    repositoryName,
    commits: commitsState.commits,
    loading: commitsState.loading,
    error: commitsState.error,
    relation,
    refreshProviderEvidence,
    contributionHistoryTarget,
  };
}
