"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { PRCommitInfo, TaskPR } from "@/lib/types/github";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import { useReviewPRSelection } from "@/hooks/domains/github/use-review-pr-selection";
import { useSessionGitStatus } from "./use-session-git-status";
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
  const gitStatus = useSessionGitStatus(sessionId ?? null);

  const relation = useMemo(
    () =>
      classifyRemoteContribution({
        hasSelectedPR: Boolean(selectedPR),
        providerCommits: commitsState.commits,
        providerLoading: commitsState.loading,
        providerError: commitsState.error,
        localHead: gitStatus?.head_commit,
        upstreamHead: gitStatus?.remote_head_commit,
        remoteAhead: gitStatus?.remote_ahead ?? 0,
        remoteBehind: gitStatus?.remote_behind ?? 0,
        baseAhead: gitStatus?.ahead ?? 0,
        hasUpstream: Boolean(gitStatus?.remote_branch),
      }),
    [selectedPR, commitsState.commits, commitsState.loading, commitsState.error, gitStatus],
  );

  return {
    selectedPR,
    commits: commitsState.commits,
    loading: commitsState.loading,
    error: commitsState.error,
    relation,
  };
}
