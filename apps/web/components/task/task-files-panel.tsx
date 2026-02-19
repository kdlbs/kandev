"use client";

import { memo } from "react";
import { TabsContent } from "@kandev/ui/tabs";
import { SessionPanel, SessionPanelContent } from "@kandev/ui/pannel-session";
import { FileBrowser } from "@/components/task/file-browser";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import type { OpenFileTab } from "@/lib/types/backend";
import {
  useFilesPanelData,
  useFilesPanelTab,
  useCommitDiffs,
  useGitStagingActions,
  useDiscardDialog,
} from "./task-files-panel-hooks";
import { DiffTabContent, DiscardDialog } from "./task-files-panel-parts";

type TaskFilesPanelProps = {
  onSelectDiff: (path: string, content?: string) => void;
  onOpenFile: (file: OpenFileTab) => void;
  activeFilePath?: string | null;
};

const TaskFilesPanel = memo(function TaskFilesPanel({
  onSelectDiff,
  onOpenFile,
  activeFilePath,
}: TaskFilesPanelProps) {
  const {
    activeSessionId,
    commits,
    changedFiles,
    reviewedCount,
    totalFileCount,
    hookDeleteFile,
    handleCreateFile,
    handleOpenFileInDocumentPanel,
    handleOpenInEditor,
  } = useFilesPanelData(onOpenFile);
  const reviewProgressPercent = totalFileCount > 0 ? (reviewedCount / totalFileCount) * 100 : 0;
  const { topTab, handleTabChange } = useFilesPanelTab(
    activeSessionId,
    changedFiles.length,
    commits.length,
  );
  const { expandedCommit, commitDiffs, loadingCommitSha, toggleCommit } =
    useCommitDiffs(activeSessionId);
  const { pendingStageFiles, handleStage, handleUnstage } = useGitStagingActions(
    activeSessionId,
    changedFiles,
  );
  const { showDiscardDialog, setShowDiscardDialog, fileToDiscard, handleDiscardClick, handleDiscardConfirm } =
    useDiscardDialog(activeSessionId);

  const tabs: SessionTab[] = [
    {
      id: "diff",
      label: `Diff files${changedFiles.length > 0 ? ` (${changedFiles.length})` : ""}`,
    },
    { id: "files", label: "All files" },
  ];

  return (
    <SessionPanel borderSide="left">
      <SessionTabs
        tabs={tabs}
        activeTab={topTab}
        onTabChange={(value) => handleTabChange(value as "diff" | "files")}
        className="flex-1 min-h-0"
      >
        <TabsContent value="diff" className="flex-1 min-h-0">
          <SessionPanelContent className="flex flex-col">
            <DiffTabContent
              changedFiles={changedFiles}
              pendingStageFiles={pendingStageFiles}
              commits={commits}
              expandedCommit={expandedCommit}
              commitDiffs={commitDiffs}
              loadingCommitSha={loadingCommitSha}
              reviewedCount={reviewedCount}
              totalFileCount={totalFileCount}
              reviewProgressPercent={reviewProgressPercent}
              onSelectDiff={onSelectDiff}
              onStage={handleStage}
              onUnstage={handleUnstage}
              onDiscard={handleDiscardClick}
              onOpenInPanel={handleOpenFileInDocumentPanel}
              onOpenInEditor={handleOpenInEditor}
              onToggleCommit={toggleCommit}
            />
          </SessionPanelContent>
        </TabsContent>
        <TabsContent value="files" className="flex-1 min-h-0">
          <SessionPanelContent>
            {activeSessionId ? (
              <FileBrowser
                sessionId={activeSessionId}
                onOpenFile={onOpenFile}
                onCreateFile={handleCreateFile}
                onDeleteFile={hookDeleteFile}
                activeFilePath={activeFilePath}
              />
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
                No task selected
              </div>
            )}
          </SessionPanelContent>
        </TabsContent>
      </SessionTabs>
      <DiscardDialog
        open={showDiscardDialog}
        onOpenChange={setShowDiscardDialog}
        fileToDiscard={fileToDiscard}
        onConfirm={handleDiscardConfirm}
      />
    </SessionPanel>
  );
});

export { TaskFilesPanel };
