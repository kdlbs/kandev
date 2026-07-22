"use client";

import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { SessionMobileTopBar } from "./session-mobile-top-bar";
import { SessionMobileBottomNav } from "./session-mobile-bottom-nav";
import { SessionTaskSwitcherSheet } from "./session-task-switcher-sheet";
import { MobileFileViewerPanel } from "./mobile-file-viewer-panel";
import { TaskChatPanel } from "../task-chat-panel";
import { TaskPlanPanel } from "../task-plan-panel";
import { MobileChangesPanel } from "./mobile-changes-panel";
import { TaskReviewDialogMount } from "../dockview-review-dialog";
import { TaskFilesPanel } from "../task-files-panel";
import { PassthroughToolbar } from "../passthrough-toolbar";
import { MobileTerminalKeybar, KEYBAR_HEIGHT_PX } from "./mobile-terminal-keybar";
import { MobileTerminalPane } from "./mobile-terminal-pane";
import { MobileSessionsPicker } from "./mobile-sessions-section";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useSessionLayoutState } from "@/hooks/use-session-layout-state";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { fetchAndOpenFile } from "../file-browser-hooks";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import type { OpenFileTab } from "@/lib/types/backend";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import {
  MRDetailPanelComponent,
  mrTaskKey,
  selectExplicitPanelMR,
} from "@/components/gitlab/mr-detail-panel";

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
  isArchived?: boolean;
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
  onOpenFile: (path: string, repo?: string) => void;
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
        <MobileSessionsPicker taskId={activeTaskId} sessionId={effectiveSessionId} fullWidth />
      </div>
      {isPassthroughMode ? (
        <div className="flex-1 min-h-0">
          <PassthroughToolbar
            key={effectiveSessionId}
            sessionId={effectiveSessionId}
            taskId={activeTaskId}
          />
        </div>
      ) : (
        <TaskChatPanel
          sessionId={effectiveSessionId}
          taskId={effectiveSessionId ? activeTaskId : null}
          onOpenFile={onOpenFile}
        />
      )}
    </div>
  );
}

type MobilePanelAreaProps = {
  currentMobilePanel: MobileSessionPanel;
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  effectiveSessionId: string | null;
  selectedFile: OpenFileTab | null;
  selectedDiff: { path: string; content?: string } | null;
  handleOpenFileFromChat: (path: string) => void;
  handleClearSelectedDiff: () => void;
  handleOpenFile: (file: OpenFileTab) => void;
  handlePanelChangeAndClearSheet: (panel: MobileSessionPanel) => void;
  topNavHeight: string;
  bottomNavHeight: string;
  mrKey?: string;
};

function MobilePanelArea({
  currentMobilePanel,
  activeTaskId,
  isPassthroughMode,
  effectiveSessionId,
  selectedFile,
  selectedDiff,
  handleOpenFileFromChat,
  handleClearSelectedDiff,
  handleOpenFile,
  handlePanelChangeAndClearSheet,
  topNavHeight,
  bottomNavHeight,
  mrKey,
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
          {selectedFile ? (
            <MobileFileViewerPanel
              key={selectedFile.path}
              file={selectedFile}
              sessionId={effectiveSessionId}
              onClose={() => handlePanelChangeAndClearSheet("files")}
            />
          ) : (
            <TaskFilesPanel onOpenFile={handleOpenFile} />
          )}
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
      {currentMobilePanel === "review" && mrKey && (
        <div className="flex min-h-0 flex-1 flex-col" data-testid="mobile-mr-review-panel">
          <MRDetailPanelComponent panelId="mobile-mr-detail" params={{ mrKey }} />
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
  isArchived?: boolean;
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
        isArchived={props.isArchived}
      />
    </div>
  );
}

export function useMobilePanelHandlers({
  effectiveSessionId,
  handlePanelChange,
}: {
  effectiveSessionId: string | null;
  handlePanelChange: (panel: MobileSessionPanel) => void;
}) {
  const { toast } = useToast();
  const [selectedFile, setSelectedFile] = useState<OpenFileTab | null>(null);
  const [trackedSessionId, setTrackedSessionId] = useState<string | null>(effectiveSessionId);
  const latestRequestIdRef = useRef(0);
  const openFileAbortRef = useRef<AbortController | null>(null);

  // Reset viewer when switching sessions — adjust state during render per
  // https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes
  if (trackedSessionId !== effectiveSessionId) {
    setTrackedSessionId(effectiveSessionId);
    setSelectedFile(null);
  }

  useLayoutEffect(() => {
    latestRequestIdRef.current += 1;
    openFileAbortRef.current?.abort();
    openFileAbortRef.current = null;
  }, [effectiveSessionId]);

  useEffect(
    () => () => {
      openFileAbortRef.current?.abort();
      openFileAbortRef.current = null;
    },
    [],
  );

  const handleOpenFileFromChat = useCallback(
    (path: string, repo?: string) => {
      if (!effectiveSessionId) return;
      const requestId = (latestRequestIdRef.current += 1);
      openFileAbortRef.current?.abort();
      const controller = new AbortController();
      openFileAbortRef.current = controller;
      void Promise.resolve(
        fetchAndOpenFile(
          effectiveSessionId,
          path,
          (file) => {
            if (requestId !== latestRequestIdRef.current || controller.signal.aborted) return;
            setSelectedFile(file);
            handlePanelChange("files");
          },
          toast,
          { repo, signal: controller.signal },
        ),
      ).finally(() => {
        if (openFileAbortRef.current === controller) {
          openFileAbortRef.current = null;
        }
      });
    },
    [effectiveSessionId, handlePanelChange, toast],
  );

  const handleOpenFile = useCallback(
    (file: OpenFileTab) => {
      latestRequestIdRef.current += 1;
      openFileAbortRef.current?.abort();
      openFileAbortRef.current = null;
      setSelectedFile(file);
      handlePanelChange("files");
    },
    [handlePanelChange],
  );

  const handlePanelChangeAndClearSheet = useCallback(
    (panel: MobileSessionPanel) => {
      latestRequestIdRef.current += 1;
      openFileAbortRef.current?.abort();
      openFileAbortRef.current = null;
      setSelectedFile(null);
      handlePanelChange(panel);
    },
    [handlePanelChange],
  );

  return {
    selectedFile,
    handleOpenFileFromChat,
    handleOpenFile,
    handlePanelChangeAndClearSheet,
  };
}

function useMobileMRSelection(
  activeTaskId: string | null,
  effectiveSessionId: string | null,
  changePanel: (panel: MobileSessionPanel) => void,
) {
  const mrs = useTaskMRs(activeTaskId);
  const reviewMRKey = useAppStore((state) =>
    effectiveSessionId
      ? (state.mobileSession.reviewMRKeyBySessionId?.[effectiveSessionId] ?? null)
      : null,
  );
  const setMobileSessionReview = useAppStore((state) => state.setMobileSessionReview);
  const selectedMR = selectExplicitPanelMR(mrs, reviewMRKey);

  useEffect(() => {
    if (effectiveSessionId && reviewMRKey && !selectedMR) {
      setMobileSessionReview(effectiveSessionId, null);
    }
  }, [effectiveSessionId, reviewMRKey, selectedMR, setMobileSessionReview]);

  const handlePanelChange = useCallback(
    (panel: MobileSessionPanel) => {
      if (panel === "review" && effectiveSessionId && !selectedMR) {
        const primaryMR = mrs[0];
        if (primaryMR) setMobileSessionReview(effectiveSessionId, mrTaskKey(primaryMR));
      }
      changePanel(panel);
    },
    [changePanel, effectiveSessionId, mrs, selectedMR, setMobileSessionReview],
  );
  return { mrs, selectedMR, handlePanelChange };
}

export const SessionMobileLayout = memo(function SessionMobileLayout(
  props: SessionMobileLayoutProps,
) {
  const {
    activeTaskId,
    effectiveSessionId,
    isPassthroughMode,
    selectedDiff,
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
  } = useSessionLayoutState({ sessionId: props.sessionId });

  const { selectedFile, handleOpenFileFromChat, handleOpenFile, handlePanelChangeAndClearSheet } =
    useMobilePanelHandlers({ effectiveSessionId, handlePanelChange });

  const mobileMR = useMobileMRSelection(
    activeTaskId,
    effectiveSessionId,
    handlePanelChangeAndClearSheet,
  );

  return (
    <div className="h-dvh relative bg-background">
      <MobileTopBarSticky
        {...props}
        activeTaskId={activeTaskId}
        effectiveSessionId={effectiveSessionId}
        onMenuClick={handleMenuClick}
        showApproveButton={showApproveButton}
        onApprove={handleApprove}
      />

      <MobilePanelArea
        currentMobilePanel={currentMobilePanel}
        activeTaskId={activeTaskId}
        isPassthroughMode={isPassthroughMode}
        effectiveSessionId={effectiveSessionId}
        selectedFile={selectedFile}
        selectedDiff={selectedDiff}
        handleOpenFileFromChat={handleOpenFileFromChat}
        handleClearSelectedDiff={handleClearSelectedDiff}
        handleOpenFile={handleOpenFile}
        handlePanelChangeAndClearSheet={handlePanelChangeAndClearSheet}
        topNavHeight={TOP_NAV_HEIGHT}
        bottomNavHeight={BOTTOM_NAV_HEIGHT}
        mrKey={mobileMR.selectedMR ? mrTaskKey(mobileMR.selectedMR) : undefined}
      />

      <MobileTerminalKeybar
        sessionId={effectiveSessionId ?? null}
        visible={currentMobilePanel === "terminal"}
        baseBottomOffset={BOTTOM_NAV_HEIGHT}
      />

      {/* Fixed Bottom Navigation */}
      <SessionMobileBottomNav
        activePanel={currentMobilePanel}
        onPanelChange={mobileMR.handlePanelChange}
        planBadge={hasUnseenPlanUpdate}
        changesBadge={totalChangesCount}
        hasReview={mobileMR.mrs.length > 0}
      />

      {/* Task Switcher Sheet */}
      <SessionTaskSwitcherSheet
        open={isTaskSwitcherOpen}
        onOpenChange={setMobileSessionTaskSwitcherOpen}
        workspaceId={props.workspaceId}
        workflowId={props.workflowId}
        presentation="drawer"
      />

      <TaskReviewDialogMount
        sessionId={effectiveSessionId}
        taskId={activeTaskId}
        onSelectWalkthroughFile={handleOpenFileFromChat}
      />
    </div>
  );
});
