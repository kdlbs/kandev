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
import type { SelectedDiff } from "../task-layout";
import type { OpenDiffOptions, DiffSheetMode } from "../changes-diff-target";

type MobileChangesPanelProps = {
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  onOpenFile?: (filePath: string) => void;
};

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
      repositoryName: options?.repositoryName,
    });
  }, []);

  const handleCloseDiffSheet = useCallback(() => {
    setDiffSheet(null);
  }, []);

  const bodyProps = buildChangesPanelBodyProps(data, {
    onOpenDiffFile: handleOpenDiffFile,
    onEditFile: onOpenFile ?? (() => {}),
    onOpenCommitDetail: (sha, repo) => {
      setDiffSheet({ kind: "commit", sha, repo });
    },
    onOpenReview: handleOpenReview,
  });

  return (
    <>
      <PanelRoot data-testid="mobile-changes-panel">
        <ChangesPanelHeader
          hasChanges={data.git.hasChanges}
          hasCommits={data.git.hasCommits}
          hasPRFiles={data.hasPRFiles}
          displayBranch={data.git.branch}
          baseBranchDisplay={data.baseBranchDisplay}
          baseBranchByRepo={data.baseBranchByRepo}
          behindCount={data.git.behind}
          isLoading={data.git.isLoading}
          loadingOperation={data.git.loadingOperation}
          onOpenDiffAll={handleOpenDiffAll}
          onOpenReview={handleOpenReview}
          repoNames={data.git.repoNames}
          perRepoStatus={data.git.perRepoStatus}
          onRepoPull={data.repoCallbacks.onRepoPull}
          onRepoRebase={data.repoCallbacks.onRepoRebase}
          onRepoMerge={data.repoCallbacks.onRepoMerge}
          repoDisplayName={data.repoDisplayName}
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
