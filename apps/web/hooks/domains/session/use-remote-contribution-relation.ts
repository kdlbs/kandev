"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { PRCommitInfo, TaskPR } from "@/lib/types/github";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import { usePRReviewRepositoryIdentity } from "@/hooks/domains/github/use-pr-review-repository-identity";
import { useReviewPRSelection } from "@/hooks/domains/github/use-review-pr-selection";
import { useSessionGitStatusByRepo } from "./use-session-git-status";
import {
  classifyRemoteContribution,
  type RemoteContributionRelation,
} from "./remote-contribution-relation";

export type RemoteContributionRelationState = {
  selectedPR: TaskPR | null;
  commits: PRCommitInfo[];
  loading: boolean;
  error: string | null;
  relation: RemoteContributionRelation;
};

export function useRemoteContributionRelation(
  sessionId: string | null | undefined,
): RemoteContributionRelationState {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const { selectedPR } = useReviewPRSelection(activeTaskId);
  const commitsState = usePRCommits(
    selectedPR?.owner ?? null,
    selectedPR?.repo ?? null,
    selectedPR?.pr_number ?? null,
    selectedPR?.last_synced_at ?? null,
  );
  const repositoryName = usePRReviewRepositoryIdentity(activeTaskId, sessionId, selectedPR);
  const statusByRepo = useSessionGitStatusByRepo(sessionId ?? null);
  const gitStatus = useMemo(() => {
    const matchingStatus = repositoryName
      ? statusByRepo.find((entry) => entry.repository_name === repositoryName)?.status
      : undefined;
    if (matchingStatus) return matchingStatus;

    // Single-repository sessions keep the legacy empty repository key even
    // when the selected PR resolves to the workspace repository name. A lone
    // empty-key entry is therefore the selected repository, while a lone
    // named entry is only a safe fallback when no identity was resolved.
    if (statusByRepo.length !== 1) return undefined;
    const onlyStatus = statusByRepo[0];
    if (onlyStatus.repository_name === "" || !repositoryName) return onlyStatus.status;
    return undefined;
  }, [repositoryName, statusByRepo]);

  const relation = useMemo(
    () =>
      classifyRemoteContribution({
        hasSelectedPR: Boolean(selectedPR),
        providerCommits: commitsState.commits,
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
      commitsState.commits,
      commitsState.providerHead,
      commitsState.providerCommitsComplete,
      commitsState.loading,
      commitsState.error,
      gitStatus,
    ],
  );

  return {
    selectedPR,
    commits: commitsState.commits,
    loading: commitsState.loading,
    error: commitsState.error,
    relation,
  };
}
