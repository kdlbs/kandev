"use client";

import { memo } from "react";
import { PanelRoot } from "./panel-primitives";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { ChangesPanelHeader } from "./changes-panel-header";
import { filterUnpushedCommits, mergeCommits } from "./changes-panel-helpers";
import { useChangesPanelData, buildChangesPanelBodyProps } from "./changes-panel-data";
import { ChangesPanelBody } from "./changes-panel-body";
import type { OpenDiffOptions } from "./changes-diff-target";
import { useRequestChangesWalkthrough } from "@/hooks/domains/session/use-request-changes-walkthrough";

export { filterUnpushedCommits, mergeCommits };

type ChangesPanelProps = {
  onOpenDiffFile: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onOpenCommitDetail?: (sha: string, repo?: string) => void;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
};

const ChangesPanel = memo(function ChangesPanel(props: ChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const data = useChangesPanelData();
  const requestWalkthrough = useRequestChangesWalkthrough({
    taskId: data.activeTaskId,
    sessionId: data.activeSessionId,
    files: data.walkthroughPromptFiles,
  });
  if (isArchived) return <ArchivedPanelPlaceholder />;
  return (
    <PanelRoot data-testid="changes-panel">
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
        onOpenDiffAll={props.onOpenDiffAll}
        onOpenReview={props.onOpenReview}
        onRequestWalkthrough={requestWalkthrough}
        requestWalkthroughDisabled={data.walkthroughPromptFiles.length === 0}
        repoNames={data.git.repoNames}
        perRepoStatus={data.git.perRepoStatus}
        onRepoPull={data.repoCallbacks.onRepoPull}
        onRepoRebase={data.repoCallbacks.onRepoRebase}
        onRepoMerge={data.repoCallbacks.onRepoMerge}
        onRenameBranch={data.git.renameBranch}
        repoDisplayName={data.repoDisplayName}
        taskId={data.activeTaskId}
      />
      <ChangesPanelBody {...buildChangesPanelBodyProps(data, props)} />
    </PanelRoot>
  );
});

export { ChangesPanel, ChangesPanelBody, useChangesPanelData, buildChangesPanelBodyProps };
export type { ChangesPanelBodyProps } from "./changes-panel-data";
