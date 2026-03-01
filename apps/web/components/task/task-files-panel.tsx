"use client";

import { memo, useMemo } from "react";
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

function FilesTabContent({
  sessionId,
  onOpenFile,
  handleCreateFile,
  hookDeleteFile,
  hookRenameFile,
  activeFilePath,
}: {
  sessionId: string | null;
  onOpenFile: (file: OpenFileTab) => void;
  handleCreateFile: (path: string) => Promise<boolean>;
  hookDeleteFile: (path: string) => Promise<boolean>;
  hookRenameFile: (oldPath: string, newPath: string) => Promise<boolean>;
  activeFilePath?: string | null;
}) {
  if (!sessionId) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
        No task selected
      </div>
    );
  }
  return (
    <FileBrowser
      sessionId={sessionId}
      onOpenFile={onOpenFile}
      onCreateFile={handleCreateFile}
      onDeleteFile={hookDeleteFile}
      onRenameFile={hookRenameFile}
      activeFilePath={activeFilePath}
    />
  );
}

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
    hookRenameFile,
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
  const {
    showDiscardDialog,
    setShowDiscardDialog,
    fileToDiscard,
    handleDiscardClick,
    handleDiscardConfirm,
  } = useDiscardDialog(activeSessionId);

  const tabs: SessionTab[] = useMemo(
    () => [
      { id: "diff", label: `Diff files${changedFiles.length > 0 ? ` (${changedFiles.length})` : ""}` },
      { id: "files", label: "All files" },
    ],
    [changedFiles.length],
  );

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
            <FilesTabContent
              sessionId={activeSessionId}
              onOpenFile={onOpenFile}
              handleCreateFile={handleCreateFile}
              hookDeleteFile={hookDeleteFile}
              hookRenameFile={hookRenameFile}
              activeFilePath={activeFilePath}
            />
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
