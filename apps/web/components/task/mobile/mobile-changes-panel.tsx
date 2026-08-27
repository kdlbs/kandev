"use client";

import { memo, useCallback, useState, useEffect, useRef } from "react";
import { PanelRoot } from "../panel-primitives";
import {
  ChangesPanelBody,
  useChangesPanelData,
  buildChangesPanelBodyProps,
} from "../changes-panel";
import { ChangesPanelHeader } from "../changes-panel-header";
import { MobileDiffSheet } from "./mobile-diff-sheet";
import { useReviewSources } from "@/hooks/domains/session/use-review-sources";
import { useAppStore } from "@/components/state-provider";
import { useRequestChangesWalkthrough } from "@/hooks/domains/session/use-request-changes-walkthrough";
import type { SelectedDiff } from "../task-layout";
import type { OpenDiffOptions, DiffSheetMode } from "../changes-diff-target";

type MobileChangesPanelProps = {
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
};

function buildContributionHeaderProps(data: ReturnType<typeof useChangesPanelData>) {
  return {
    relation: data.relation,
    resolution: data.resolution,
    resolutionTarget: data.resolutionTarget,
    remoteContributionUrl: data.selectedPR?.pr_url ?? data.existingPrUrl,
    remoteContributionNumber: data.selectedPR?.pr_number,
  };
}

/**
 * Mobile Changes panel — renders the same timeline summary surface as desktop.
 * Reuses ChangesPanelBody + ChangesPanelHeader from desktop.
 * Wires [Diff] / [Review] / row taps into mobile overlays (Drawer/Sheet).
 */
export const MobileChangesPanel = memo(function MobileChangesPanel({
  selectedDiff,
  onClearSelected,
  onOpenFile,
}: MobileChangesPanelProps) {
  const data = useChangesPanelData();
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const { sourceCounts } = useReviewSources(activeSessionId);
  const [diffSheet, setDiffSheet] = useState<DiffSheetMode | null>(null);
  const requestWalkthrough = useRequestChangesWalkthrough({
    taskId: data.activeTaskId,
    sessionId: data.activeSessionId,
    ready: data.walkthroughRequestReady,
  });

  // Track the previous selectedDiff to detect changes
  const prevSelectedDiffRef = useRef<SelectedDiff | null>(null);
  useEffect(() => {
    // Only open the sheet if selectedDiff changed from null/undefined to a new path
    if (!selectedDiff?.path) {
      prevSelectedDiffRef.current = selectedDiff;
      return;
    }

    const prevPath = prevSelectedDiffRef.current?.path;
    prevSelectedDiffRef.current = selectedDiff;
    if (prevPath === selectedDiff.path) return;

    // queueMicrotask satisfies react-hooks/set-state-in-effect; executes before next paint.
    queueMicrotask(() => {
      setDiffSheet({ kind: "file", path: selectedDiff.path });
      onClearSelected();
    });
  }, [selectedDiff, onClearSelected]);

  const handleOpenDiffAll = useCallback(() => {
    setDiffSheet({ kind: "all" });
  }, []);

  const handleOpenReview = useCallback(() => {
    window.dispatchEvent(new CustomEvent("open-review-dialog"));
  }, []);

  const handleOpenDiffFile = useCallback((path: string, options?: OpenDiffOptions) => {
    setDiffSheet({
      kind: "file",
      path,
      sourceFilter: options?.source ?? "all",
      repositoryName: options?.repositoryName || undefined,
      prKey: options?.prKey,
    });
  }, []);

  const handleCloseDiffSheet = useCallback(() => {
    setDiffSheet(null);
  }, []);

  const bodyProps = buildChangesPanelBodyProps(data, {
    onOpenDiffFile: handleOpenDiffFile,
    onEditFile: onOpenFile ?? (() => {}),
    onOpenCommitDetail: (target) => {
      setDiffSheet({ kind: "commit", target });
    },
    onOpenReview: handleOpenReview,
  });

  return (
    <>
      <PanelRoot className="@container/changes-panel" data-testid="mobile-changes-panel">
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
          onOpenDiffAll={handleOpenDiffAll}
          onOpenReview={handleOpenReview}
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
          {...buildContributionHeaderProps(data)}
        />
        <ChangesPanelBody {...bodyProps} />
      </PanelRoot>

      <MobileDiffSheet
        mode={diffSheet}
        onClose={handleCloseDiffSheet}
        onOpenFile={onOpenFile}
        selectedDiff={selectedDiff}
        onClearSelected={onClearSelected}
        sourceCounts={sourceCounts}
      />
    </>
  );
});
