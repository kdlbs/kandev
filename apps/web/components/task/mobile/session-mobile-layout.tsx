"use client";

import { memo, useCallback } from "react";
import { SessionMobileTopBar } from "./session-mobile-top-bar";
import { SessionMobileBottomNav } from "./session-mobile-bottom-nav";
import { SessionTaskSwitcherSheet } from "./session-task-switcher-sheet";
import { TaskChatPanel } from "../task-chat-panel";
import { TaskPlanPanel } from "../task-plan-panel";
import { TaskChangesPanel } from "../task-changes-panel";
import { TaskFilesPanel } from "../task-files-panel";
import { ShellTerminal } from "../shell-terminal";
import { PassthroughTerminal } from "../passthrough-terminal";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useSessionLayoutState } from "@/hooks/use-session-layout-state";
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
  sessionId,
  onOpenFile,
}: {
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  effectiveSessionId: string | null;
  sessionId?: string | null;
  onOpenFile: (path: string) => void;
}) {
  if (activeTaskId && isPassthroughMode) {
    return (
      <div className="flex-1 min-h-0">
        <PassthroughTerminal key={effectiveSessionId} sessionId={sessionId} mode="agent" />
      </div>
    );
  }
  if (activeTaskId) {
    return <TaskChatPanel sessionId={sessionId} onOpenFile={onOpenFile} />;
  }
  return (
    <div className="flex-1 flex items-center justify-center text-muted-foreground">
      No task selected
    </div>
  );
}

type MobilePanelAreaProps = {
  currentMobilePanel: MobileSessionPanel;
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  effectiveSessionId: string | null;
  sessionId?: string | null;
  selectedDiff: { path: string; content?: string } | null;
  handleOpenFileFromChat: (path: string) => void;
  handleClearSelectedDiff: () => void;
  handleSelectDiffAndSwitchPanel: (path: string, content?: string) => void;
  handleOpenFile: (file: { path: string }) => void;
  topNavHeight: string;
  bottomNavHeight: string;
};

function MobilePanelArea({
  currentMobilePanel,
  activeTaskId,
  isPassthroughMode,
  effectiveSessionId,
  sessionId,
  selectedDiff,
  handleOpenFileFromChat,
  handleClearSelectedDiff,
  handleSelectDiffAndSwitchPanel,
  handleOpenFile,
  topNavHeight,
  bottomNavHeight,
}: MobilePanelAreaProps) {
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
        <div className="flex-1 min-h-0 flex flex-col p-2">
          <MobileChatPanelContent
            activeTaskId={activeTaskId}
            isPassthroughMode={isPassthroughMode}
            effectiveSessionId={effectiveSessionId}
            sessionId={sessionId}
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
          <TaskChangesPanel
            selectedDiff={selectedDiff}
            onClearSelected={handleClearSelectedDiff}
            onOpenFile={handleOpenFileFromChat}
          />
        </div>
      )}
      {currentMobilePanel === "files" && (
        <div className="flex-1 min-h-0 flex flex-col">
          <TaskFilesPanel
            onSelectDiff={handleSelectDiffAndSwitchPanel}
            onOpenFile={handleOpenFile}
          />
        </div>
      )}
      {currentMobilePanel === "terminal" && (
        <div className="flex-1 min-h-0 flex flex-col p-2">
          <SessionPanelContent className="p-0 flex-1 min-h-0">
            <ShellTerminal key={effectiveSessionId} sessionId={effectiveSessionId ?? undefined} />
          </SessionPanelContent>
        </div>
      )}
    </div>
  );
}

function useMobilePanelHandlers({
  handleSelectDiff,
  handlePanelChange,
}: {
  handleSelectDiff: (path: string, content?: string) => void;
  handlePanelChange: (panel: MobileSessionPanel) => void;
}) {
  const handleSelectDiffAndSwitchPanel = useCallback(
    (path: string, content?: string) => {
      handleSelectDiff(path, content);
      handlePanelChange("changes");
    },
    [handleSelectDiff, handlePanelChange],
  );

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

  return { handleSelectDiffAndSwitchPanel, handleOpenFileFromChat, handleOpenFile };
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

  const { handleSelectDiffAndSwitchPanel, handleOpenFileFromChat, handleOpenFile } =
    useMobilePanelHandlers({
      handleSelectDiff,
      handlePanelChange,
    });

  return (
    <div className="h-dvh relative bg-background">
      {/* Fixed Top Bar */}
      <div
        className="fixed top-0 left-0 right-0 z-40 bg-background border-b border-border"
        style={{ paddingTop: "env(safe-area-inset-top, 0px)" }}
      >
        <SessionMobileTopBar
          taskTitle={taskTitle}
          sessionId={effectiveSessionId}
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
      </div>

      {/* Content Area - fixed height panels that manage their own scrolling */}
      <MobilePanelArea
        currentMobilePanel={currentMobilePanel}
        activeTaskId={activeTaskId}
        isPassthroughMode={isPassthroughMode}
        effectiveSessionId={effectiveSessionId}
        sessionId={sessionId}
        selectedDiff={selectedDiff}
        handleOpenFileFromChat={handleOpenFileFromChat}
        handleClearSelectedDiff={handleClearSelectedDiff}
        handleSelectDiffAndSwitchPanel={handleSelectDiffAndSwitchPanel}
        handleOpenFile={handleOpenFile}
        topNavHeight={TOP_NAV_HEIGHT}
        bottomNavHeight={BOTTOM_NAV_HEIGHT}
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
    </div>
  );
});
