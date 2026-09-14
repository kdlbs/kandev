"use client";

import { memo, useEffect, useMemo } from "react";
import { PanelRoot } from "./panel-primitives";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { ChangesPanelHeader } from "./changes-panel-header";
import {
  filterUnpushedCommits,
  mergeCommits,
  separateCommitHistories,
} from "./changes-panel-helpers";
import { useChangesPanelData, buildChangesPanelBodyProps } from "./changes-panel-data";
import { ChangesPanelBody } from "./changes-panel-body";
import type { CommitDetailTarget, OpenDiffOptions } from "./changes-diff-target";
import { useRequestChangesWalkthrough } from "@/hooks/domains/session/use-request-changes-walkthrough";
import {
  consumeContributionComparisonRequest,
  useContributionComparisonRequest,
} from "./remote-contribution-comparison";
import { contributionHistoryExplanationKey } from "@/hooks/domains/session/use-contribution-history-explanation";

export { filterUnpushedCommits, mergeCommits, separateCommitHistories };

type ChangesPanelProps = {
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (target: CommitDetailTarget) => void;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
};

const ChangesPanel = memo(function ChangesPanel(props: ChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const data = useChangesPanelData();
  const requestWalkthrough = useRequestChangesWalkthrough({
    taskId: data.activeTaskId,
    sessionId: data.activeSessionId,
    ready: data.walkthroughRequestReady,
  });
  const comparisonRequest = useContributionComparisonRequest();
  const contributionKey = useMemo(
    () => contributionHistoryExplanationKey(data.contributionHistoryTarget),
    [data.contributionHistoryTarget],
  );
  const comparisonRequestToken =
    comparisonRequest?.key === contributionKey ? comparisonRequest.token : undefined;
  useEffect(() => {
    if (comparisonRequestToken !== undefined) {
      // Let the targeted disclosure's focus effect run before consuming the
      // one-shot request. The second frame also gives the menu/drawer close
      // primitive time to finish its own focus bookkeeping.
      let consumeFrame = 0;
      const settleFrame = requestAnimationFrame(() => {
        consumeFrame = requestAnimationFrame(() => {
          consumeContributionComparisonRequest(comparisonRequestToken);
        });
      });
      return () => {
        cancelAnimationFrame(settleFrame);
        if (consumeFrame) cancelAnimationFrame(consumeFrame);
      };
    }
  }, [comparisonRequestToken]);
  if (isArchived) return <ArchivedPanelPlaceholder />;
  return (
    <PanelRoot className="@container/changes-panel" data-testid="changes-panel">
      <ChangesPanelHeader
        hasChanges={data.git.hasChanges}
        hasCommits={data.git.hasCommits}
        hasPRFiles={data.hasPRFiles}
        displayBranch={data.git.branch}
        baseBranchDisplay={data.baseBranchDisplay}
        baseBranchByRepo={data.baseBranchByRepo}
        behindCount={data.git.pullBehind}
        pullDisabled={data.pullDisabled}
        pullDisabledReason={data.pullDisabledReason}
        isLoading={data.git.isLoading}
        loadingOperation={data.git.loadingOperation}
        onOpenDiffAll={props.onOpenDiffAll}
        onOpenReview={props.onOpenReview}
        onRequestWalkthrough={requestWalkthrough}
        requestWalkthroughDisabled={!data.walkthroughRequestReady}
        repoNames={data.git.repoNames}
        perRepoStatus={data.git.perRepoStatus}
        onRepoPull={data.repoCallbacks.onRepoPull}
        onRepoRebase={data.repoCallbacks.onRepoRebase}
        onRepoMerge={data.repoCallbacks.onRepoMerge}
        onRenameBranch={data.git.renameBranch}
        repoDisplayName={data.repoDisplayName}
        taskId={data.activeTaskId}
        credentialDisplay={data.gitCredentialDisplay}
        comparisonTargets={data.git.comparisonTargets}
        relation={data.relation}
        contributionHistoryTarget={data.contributionHistoryTarget}
        resolution={data.resolution}
        resolutionTarget={data.resolutionTarget}
        remoteContributionUrl={data.selectedPR?.pr_url ?? data.existingPrUrl}
        remoteContributionNumber={data.selectedPR?.pr_number}
      />
      <ChangesPanelBody
        {...buildChangesPanelBodyProps(data, props)}
        comparisonRequestToken={comparisonRequestToken}
      />
    </PanelRoot>
  );
});

export { ChangesPanel, ChangesPanelBody, useChangesPanelData, buildChangesPanelBodyProps };
export type { ChangesPanelBodyProps } from "./changes-panel-data";
