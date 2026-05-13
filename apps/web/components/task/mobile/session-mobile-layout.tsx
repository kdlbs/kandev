"use client";

import { memo, useCallback } from "react";
import { SessionMobileTopBar } from "./session-mobile-top-bar";
import { SessionMobileBottomNav } from "./session-mobile-bottom-nav";
import { SessionTaskSwitcherSheet } from "./session-task-switcher-sheet";
import { TaskChatPanel } from "../task-chat-panel";
import { TaskPlanPanel } from "../task-plan-panel";
import { MobileChangesPanel } from "./mobile-changes-panel";
import { ReviewDialog } from "@/components/review/review-dialog";
import { useReviewDialog } from "../use-review-dialog";
import { TaskFilesPanel } from "../task-files-panel";
import { PassthroughTerminal } from "../passthrough-terminal";
import { MobileTerminalKeybar, KEYBAR_HEIGHT_PX } from "./mobile-terminal-keybar";
import { MobileTerminalPane } from "./mobile-terminal-pane";
import { MobileSessionsPicker } from "./mobile-sessions-section";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useSessionLayoutState } from "@/hooks/use-session-layout-state";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";

const TOP_NAV_HEIGHT = "3.5rem";
const BOTTOM_NAV_HEIGHT = "3.25rem";

type SessionMobileLayoutProps = {
  workspaceId: string | null;
  workflowId: string | null;
  sessionId?: string | null;
  baseBranch?: string;
  worktreeBranch?: string | null;
  taskTitle?: string;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string | null;
  remoteExecutorName?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
};

function MobileChatPanelContent({
  activeTaskId,
  isPassthroughMode,
  effectiveSessionId,
  onOpenFile,
}: {
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  effectiveSessionId: string | null;
  onOpenFile: (path: string) => void;
}) {
  if (!activeTaskId) {
    return (
      <div className="flex-1 flex items-center justify-center text-muted-foreground">
        No task selected
      </div>
    );
  }
  return (
    <div className="flex-1 min-h-0 flex flex-col">
      <div className="flex items-center px-1 py-2">
        <MobileSessionsPicker taskId={activeTaskId} fullWidth />
      </div>
      {isPassthroughMode ? (
        <div className="flex-1 min-h-0">
          <PassthroughTerminal
            key={effectiveSessionId}
            sessionId={effectiveSessionId}
            mode="agent"
          />
        </div>
      ) : (
        <TaskChatPanel sessionId={effectiveSessionId} onOpenFile={onOpenFile} />
      )}
    </div>
  );
}

type MobilePanelAreaProps = {
  currentMobilePanel: MobileSessionPanel;
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  effectiveSessionId: string | null;
  selectedDiff: { path: string; content?: string } | null;
  handleOpenFileFromChat: (path: string) => void;
  handleClearSelectedDiff: () => void;
  handleOpenFile: (file: { path: string }) => void;
  topNavHeight: string;
  bottomNavHeight: string;
};

function MobilePanelArea({
  currentMobilePanel,
  activeTaskId,
  isPassthroughMode,
  effectiveSessionId,
  selectedDiff,
  handleOpenFileFromChat,
  handleClearSelectedDiff,
  handleOpenFile,
  topNavHeight,
  bottomNavHeight,
}: MobilePanelAreaProps) {
  const { keyboardOpen, bottomOffset } = useVisualViewportOffset();
  // Keep terminal content's visible bottom glued to the keybar top. When the
  // keyboard is up, the content area already pads for the bottom nav (which
  // is now under the keyboard), so we subtract it back out and add the
  // keyboard height instead.
  const terminalPaddingBottom = keyboardOpen
    ? `calc(${bottomOffset + KEYBAR_HEIGHT_PX}px - ${bottomNavHeight} - env(safe-area-inset-bottom, 0px))`
    : `${KEYBAR_HEIGHT_PX}px`;
  return (
    <div
      className="flex flex-col"
      style={{
        paddingTop: `calc(${topNavHeight} + env(safe-area-inset-top, 0px))`,
        paddingBottom: `calc(${bottomNavHeight} + env(safe-area-inset-bottom, 0px))`,
        height: "100dvh",
      }}
    >
      {currentMobilePanel === "chat" && (
        <div className="flex-1 min-h-0 flex flex-col px-2 pb-2">
          <MobileChatPanelContent
            activeTaskId={activeTaskId}
            isPassthroughMode={isPassthroughMode}
            effectiveSessionId={effectiveSessionId}
            onOpenFile={handleOpenFileFromChat}
          />
        </div>
      )}
      {currentMobilePanel === "plan" && (
        <div className="flex-1 min-h-0 flex flex-col p-2">
          <TaskPlanPanel taskId={activeTaskId} visible={true} />
        </div>
      )}
      {currentMobilePanel === "changes" && (
        <div className="flex-1 min-h-0 flex flex-col p-2">
          <MobileChangesPanel
            selectedDiff={selectedDiff}
            onClearSelected={handleClearSelectedDiff}
            onOpenFile={handleOpenFileFromChat}
          />
        </div>
      )}
      {currentMobilePanel === "files" && (
        <div className="flex-1 min-h-0 flex flex-col">
          <TaskFilesPanel onOpenFile={handleOpenFile} />
        </div>
      )}
      {currentMobilePanel === "terminal" && (
        <div
          data-testid="terminal-panel"
          className="flex-1 min-h-0 flex flex-col px-2"
          style={{ paddingBottom: terminalPaddingBottom }}
        >
          <SessionPanelContent className="p-0 flex-1 min-h-0 flex flex-col">
            <MobileTerminalPane key={effectiveSessionId} sessionId={effectiveSessionId} />
          </SessionPanelContent>
        </div>
      )}
    </div>
  );
}

type MobileTopBarStickyProps = {
  activeTaskId: string | null;
  workspaceId: string | null;
  taskTitle?: string;
  effectiveSessionId: string | null;
  baseBranch?: string;
  worktreeBranch?: string | null;
  onMenuClick: () => void;
  showApproveButton: boolean;
  onApprove: () => void;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string | null;
  remoteExecutorName?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
};

function MobileTopBarSticky(props: MobileTopBarStickyProps) {
  return (
    <div
      className="fixed top-0 left-0 right-0 z-40 bg-background border-b border-border"
      style={{ paddingTop: "env(safe-area-inset-top, 0px)" }}
    >
      <SessionMobileTopBar
        taskId={props.activeTaskId}
        workspaceId={props.workspaceId}
        taskTitle={props.taskTitle}
        sessionId={props.effectiveSessionId}
        baseBranch={props.baseBranch}
        worktreeBranch={props.worktreeBranch}
        onMenuClick={props.onMenuClick}
        showApproveButton={props.showApproveButton}
        onApprove={props.onApprove}
        isRemoteExecutor={props.isRemoteExecutor}
        remoteExecutorType={props.remoteExecutorType}
        remoteExecutorName={props.remoteExecutorName}
        remoteState={props.remoteState}
        remoteCreatedAt={props.remoteCreatedAt}
        remoteCheckedAt={props.remoteCheckedAt}
        remoteStatusError={props.remoteStatusError}
      />
    </div>
  );
}

function MobileReviewDialogMount({
  sessionId,
  review,
}: {
  sessionId: string | null;
  review: ReturnType<typeof useReviewDialog>;
}) {
  if (!sessionId) return null;
  return (
    <ReviewDialog
      open={review.reviewDialogOpen}
      onOpenChange={review.setReviewDialogOpen}
      sessionId={sessionId}
      baseBranch={review.baseBranch}
      onSendComments={review.handleReviewSendComments}
      onOpenFile={review.reviewOpenFile}
      gitStatusFiles={review.reviewGitStatusFiles}
      cumulativeDiff={review.reviewCumulativeDiff}
      prDiffFiles={review.reviewPRDiffFiles}
    />
  );
}

function useMobilePanelHandlers({
  handleSelectDiff,
  handlePanelChange,
}: {
  handleSelectDiff: (path: string, content?: string) => void;
  handlePanelChange: (panel: MobileSessionPanel) => void;
}) {
  const handleOpenFileFromChat = useCallback(
    (path: string) => {
      handleSelectDiff(path);
      handlePanelChange("changes");
    },
    [handleSelectDiff, handlePanelChange],
  );

  const handleOpenFile = useCallback(
    (file: { path: string }) => {
      handleSelectDiff(file.path);
      handlePanelChange("changes");
    },
    [handleSelectDiff, handlePanelChange],
  );

  return { handleOpenFileFromChat, handleOpenFile };
}

export const SessionMobileLayout = memo(function SessionMobileLayout({
  workspaceId,
  workflowId,
  sessionId,
  baseBranch,
  worktreeBranch,
  taskTitle,
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: SessionMobileLayoutProps) {
  // Use shared layout state hook
  const {
    activeTaskId,
    effectiveSessionId,
    isPassthroughMode,
    selectedDiff,
    handleSelectDiff,
    handleClearSelectedDiff,
    totalChangesCount,
    hasUnseenPlanUpdate,
    showApproveButton,
    handleApprove,
    currentMobilePanel,
    handlePanelChange,
    isTaskSwitcherOpen,
    handleMenuClick,
    setMobileSessionTaskSwitcherOpen,
  } = useSessionLayoutState({ sessionId });

  const { handleOpenFileFromChat, handleOpenFile } = useMobilePanelHandlers({
    handleSelectDiff,
    handlePanelChange,
  });

  const review = useReviewDialog(effectiveSessionId);

  return (
    <div className="h-dvh relative bg-background">
      <MobileTopBarSticky
        activeTaskId={activeTaskId}
        workspaceId={workspaceId}
        taskTitle={taskTitle}
        effectiveSessionId={effectiveSessionId}
        baseBranch={baseBranch}
        worktreeBranch={worktreeBranch}
        onMenuClick={handleMenuClick}
        showApproveButton={showApproveButton}
        onApprove={handleApprove}
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorType={remoteExecutorType}
        remoteExecutorName={remoteExecutorName}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
      />

      {/* Content Area - fixed height panels that manage their own scrolling */}
      <MobilePanelArea
        currentMobilePanel={currentMobilePanel}
        activeTaskId={activeTaskId}
        isPassthroughMode={isPassthroughMode}
        effectiveSessionId={effectiveSessionId}
        selectedDiff={selectedDiff}
        handleOpenFileFromChat={handleOpenFileFromChat}
        handleClearSelectedDiff={handleClearSelectedDiff}
        handleOpenFile={handleOpenFile}
        topNavHeight={TOP_NAV_HEIGHT}
        bottomNavHeight={BOTTOM_NAV_HEIGHT}
      />

      {/* Terminal key-bar — docks above OS keyboard or bottom nav on the terminal panel */}
      <MobileTerminalKeybar
        sessionId={effectiveSessionId ?? null}
        visible={currentMobilePanel === "terminal"}
        baseBottomOffset={BOTTOM_NAV_HEIGHT}
      />

      {/* Fixed Bottom Navigation */}
      <SessionMobileBottomNav
        activePanel={currentMobilePanel}
        onPanelChange={handlePanelChange}
        planBadge={hasUnseenPlanUpdate}
        changesBadge={totalChangesCount}
      />

      {/* Task Switcher Sheet */}
      <SessionTaskSwitcherSheet
        open={isTaskSwitcherOpen}
        onOpenChange={setMobileSessionTaskSwitcherOpen}
        workspaceId={workspaceId}
        workflowId={workflowId}
      />

      <MobileReviewDialogMount sessionId={effectiveSessionId} review={review} />
    </div>
  );
});
