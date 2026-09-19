"use client";

import { PanelHeaderBarSplit } from "./panel-primitives";
import type { GitCredentialDisplay } from "./changes-git-credential-display";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import type { ContributionHistoryExplanationTarget } from "@/hooks/domains/session/use-contribution-history-explanation";
import {
  type RemoteContributionResolutionTarget,
  type useRemoteContributionResolution,
} from "./use-remote-contribution-resolution";
import { RemoteContributionHeaderActions } from "./remote-contribution-header-actions";
import { buildBranchRows, type PerRepoStatus } from "./changes-panel-branch-rows";
import {
  BranchHoverCard,
  ChangesPanelHeaderLeft,
  ChangesPanelHeaderOverflowActions,
  PullDropdown,
} from "./changes-panel-header-actions";
import type { RenameBranchResult } from "./changes-panel-header-actions";

export { PullDropdown } from "./changes-panel-header-actions";

export type { PerRepoStatus } from "./changes-panel-branch-rows";

function buildHeaderBranchRows(props: ChangesPanelHeaderProps) {
  return buildBranchRows(
    props.perRepoStatus,
    props.baseBranchByRepo,
    props.baseBranchDisplay,
    props.repoDisplayName,
  );
}

type ChangesPanelHeaderProps = {
  hasChanges: boolean;
  hasCommits: boolean;
  hasPRFiles?: boolean;
  displayBranch: string | null;
  baseBranchDisplay: string;
  /** Per-repo merge target, keyed by repository_name. Undefined entries fall
   *  back to baseBranchDisplay. Empty/missing for single-repo workspaces. */
  baseBranchByRepo?: Record<string, string>;
  behindCount: number;
  pullDisabled?: boolean;
  pullDisabledReason?: string;
  isLoading: boolean;
  loadingOperation: string | null;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  credentialDisplay: GitCredentialDisplay | null;
  comparisonTargets: string[];
  repoDisplayName?: (repositoryName: string) => string | undefined;
  /** Active task id; piped into the base-branch picker so it can resolve
   *  the right task_repositories row to PATCH. Null while task data is
   *  hydrating — the picker falls back to a static label. */
  taskId: string | null;
  relation?: RemoteContributionRelation;
  contributionHistoryTarget?: ContributionHistoryExplanationTarget | null;
  resolution?: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget?: RemoteContributionResolutionTarget | null;
  remoteContributionUrl?: string;
  remoteContributionNumber?: number;
};

function ChangesPanelHeaderRight({
  props,
  branchRows,
}: {
  props: ChangesPanelHeaderProps;
  branchRows: ReturnType<typeof buildHeaderBranchRows>;
}) {
  const {
    displayBranch,
    baseBranchDisplay,
    taskId,
    onRenameBranch,
    loadingOperation,
    credentialDisplay,
    comparisonTargets,
    relation,
    contributionHistoryTarget,
    resolution,
    resolutionTarget,
    remoteContributionUrl,
    remoteContributionNumber,
    behindCount,
    pullDisabled,
    pullDisabledReason,
    isLoading,
    repoNames,
    perRepoStatus,
    onRepoPull,
    onRepoRebase,
    onRepoMerge,
    repoDisplayName,
  } = props;
  return (
    <>
      {(displayBranch || branchRows.length > 0) && (
        <BranchHoverCard
          displayBranch={displayBranch ?? ""}
          baseBranchDisplay={baseBranchDisplay}
          rows={branchRows}
          taskId={taskId}
          onRenameBranch={onRenameBranch}
          isRenaming={loadingOperation === "rename_branch"}
          credentialDisplay={credentialDisplay}
          comparisonTargets={comparisonTargets}
        />
      )}
      <RemoteContributionHeaderActions
        relation={relation}
        contributionHistoryTarget={contributionHistoryTarget}
        resolution={resolution}
        resolutionTarget={resolutionTarget}
        prUrl={remoteContributionUrl}
        prNumber={remoteContributionNumber}
      />
      <PullDropdown
        behindCount={behindCount}
        pullDisabled={pullDisabled}
        pullDisabledReason={pullDisabledReason}
        isLoading={isLoading}
        loadingOperation={loadingOperation}
        repoNames={repoNames}
        perRepoStatus={perRepoStatus}
        onRepoPull={onRepoPull}
        onRepoRebase={onRepoRebase}
        onRepoMerge={onRepoMerge}
        repoDisplayName={repoDisplayName}
      />
    </>
  );
}

export function ChangesPanelHeader(props: ChangesPanelHeaderProps) {
  const {
    hasChanges,
    hasCommits,
    hasPRFiles,
    onOpenDiffAll,
    onOpenReview,
    onRequestWalkthrough,
    requestWalkthroughDisabled,
  } = props;
  const branchRows = buildHeaderBranchRows(props);
  const showDiffReview = hasChanges || hasCommits || !!hasPRFiles;
  return (
    <PanelHeaderBarSplit
      left={
        <ChangesPanelHeaderLeft
          showDiffReview={showDiffReview}
          onOpenDiffAll={onOpenDiffAll}
          onOpenReview={onOpenReview}
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      leftWhenOverflow={
        <ChangesPanelHeaderLeft
          showDiffReview={showDiffReview}
          primaryOnly
          onOpenReview={onOpenReview}
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      right={<ChangesPanelHeaderRight props={props} branchRows={branchRows} />}
      overflow={
        <ChangesPanelHeaderOverflowActions
          showDiffReview={showDiffReview}
          onOpenDiffAll={onOpenDiffAll}
          onOpenReview={onOpenReview}
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      overflowAt={520}
      hideLeftWhenOverflow
      hideRightWhenOverflow={false}
    />
  );
}
