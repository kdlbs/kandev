'use client';

import { memo, useCallback } from 'react';
import { SessionMobileTopBar } from './session-mobile-top-bar';
import { SessionMobileBottomNav } from './session-mobile-bottom-nav';
import { SessionTaskSwitcherSheet } from './session-task-switcher-sheet';
import { TaskChatPanel } from '../task-chat-panel';
import { TaskPlanPanel } from '../task-plan-panel';
import { TaskChangesPanel } from '../task-changes-panel';
import { TaskFilesPanel } from '../task-files-panel';
import { ShellTerminal } from '../shell-terminal';
import { PassthroughTerminal } from '../passthrough-terminal';
import { SessionPanelContent } from '@kandev/ui/pannel-session';
import { useSessionLayoutState } from '@/hooks/use-session-layout-state';

type SessionMobileLayoutProps = {
  workspaceId: string | null;
  workflowId: string | null;
  sessionId?: string | null;
  baseBranch?: string;
  worktreeBranch?: string | null;
  taskTitle?: string;
};

export const SessionMobileLayout = memo(function SessionMobileLayout({
  workspaceId,
  workflowId,
  sessionId,
  baseBranch,
  worktreeBranch,
  taskTitle,
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

  // Mobile-specific handlers that also switch panels
  const handleSelectDiffAndSwitchPanel = useCallback((path: string, content?: string) => {
    handleSelectDiff(path, content);
    handlePanelChange('changes');
  }, [handleSelectDiff, handlePanelChange]);

  const handleOpenFileFromChat = useCallback((path: string) => {
    handleSelectDiff(path);
    handlePanelChange('changes');
  }, [handleSelectDiff, handlePanelChange]);

  const handleOpenFile = useCallback((file: { path: string }) => {
    handleSelectDiff(file.path);
    handlePanelChange('changes');
  }, [handleSelectDiff, handlePanelChange]);

  // Top nav height (~56px) + bottom nav height (~52px)
  const topNavHeight = '3.5rem';
  const bottomNavHeight = '3.25rem';

  return (
    <div className="h-dvh relative bg-background">
      {/* Fixed Top Bar */}
      <div
        className="fixed top-0 left-0 right-0 z-40 bg-background border-b border-border"
        style={{ paddingTop: 'env(safe-area-inset-top, 0px)' }}
      >
        <SessionMobileTopBar
          taskTitle={taskTitle}
          sessionId={effectiveSessionId}
          baseBranch={baseBranch}
          worktreeBranch={worktreeBranch}
          onMenuClick={handleMenuClick}
          showApproveButton={showApproveButton}
          onApprove={handleApprove}
        />
      </div>

      {/* Content Area - fixed height panels that manage their own scrolling */}
      <div
        className="flex flex-col"
        style={{
          paddingTop: `calc(${topNavHeight} + env(safe-area-inset-top, 0px))`,
          paddingBottom: `calc(${bottomNavHeight} + env(safe-area-inset-bottom, 0px))`,
          height: '100dvh',
        }}
      >
        {currentMobilePanel === 'chat' && (
          <div className="flex-1 min-h-0 flex flex-col p-2">
            {activeTaskId ? (
              isPassthroughMode ? (
                <div className="flex-1 min-h-0">
                  <PassthroughTerminal key={effectiveSessionId} sessionId={sessionId} mode="agent" />
                </div>
              ) : (
                <TaskChatPanel
                  sessionId={sessionId}
                  onOpenFile={handleOpenFileFromChat}
                />
              )
            ) : (
              <div className="flex-1 flex items-center justify-center text-muted-foreground">
                No task selected
              </div>
            )}
          </div>
        )}

        {currentMobilePanel === 'plan' && (
          <div className="flex-1 min-h-0 flex flex-col p-2">
            <TaskPlanPanel taskId={activeTaskId} visible={true} />
          </div>
        )}

        {currentMobilePanel === 'changes' && (
          <div className="flex-1 min-h-0 flex flex-col p-2">
            <TaskChangesPanel
              selectedDiff={selectedDiff}
              onClearSelected={handleClearSelectedDiff}
              onOpenFile={handleOpenFileFromChat}
            />
          </div>
        )}

        {currentMobilePanel === 'files' && (
          <div className="flex-1 min-h-0 flex flex-col">
            <TaskFilesPanel
              onSelectDiff={handleSelectDiffAndSwitchPanel}
              onOpenFile={handleOpenFile}
            />
          </div>
        )}

        {currentMobilePanel === 'terminal' && (
          <div className="flex-1 min-h-0 flex flex-col p-2">
            <SessionPanelContent className="p-0 flex-1 min-h-0">
              <ShellTerminal key={effectiveSessionId} sessionId={effectiveSessionId ?? undefined} />
            </SessionPanelContent>
          </div>
        )}
      </div>

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
