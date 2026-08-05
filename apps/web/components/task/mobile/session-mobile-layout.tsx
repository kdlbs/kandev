"use client";

import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { SessionMobileTopBar } from "./session-mobile-top-bar";
import { SessionMobileBottomNav } from "./session-mobile-bottom-nav";
import { SessionTaskSwitcherSheet } from "./session-task-switcher-sheet";
import { MobileFileViewerPanel } from "./mobile-file-viewer-panel";
import { TaskChatPanel } from "../task-chat-panel";
import { TaskPlanPanel } from "../task-plan-panel";
import { MobileChangesPanel } from "./mobile-changes-panel";
import { SessionMobileReviewDialog } from "./session-mobile-review-dialog";
import { TaskFilesPanel } from "../task-files-panel";
import { PassthroughToolbar } from "../passthrough-toolbar";
import { MobileTerminalKeybar, KEYBAR_HEIGHT_PX } from "./mobile-terminal-keybar";
import { MobileTerminalPane } from "./mobile-terminal-pane";
import { MobileSessionsPicker } from "./mobile-sessions-section";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useSessionLayoutState } from "@/hooks/use-session-layout-state";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";
import { useToast } from "@/components/toast-provider";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { fetchAndOpenFile } from "../file-browser-hooks";
import { useMobileMRSelection } from "./use-mobile-mr-selection";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import type { OpenFileTab } from "@/lib/types/backend";
import { useReviewPRSelection } from "@/hooks/domains/github/use-review-pr-selection";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { prTaskKey } from "@/components/github/pr-utils";
import { ReviewPRSelector } from "@/components/review/review-pr-selector";
import { MRDetailPanelComponent, mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { PluginTaskPanel } from "../plugin-task-panel";
import { parsePluginPanelId } from "@/lib/state/layout-manager/plugin-panels";
import { useEffectiveMobilePanel, type MobileReviewSource } from "./mobile-plugin-panel-lifecycle";

export { resolveMobilePluginPanel } from "./mobile-plugin-panel-lifecycle";

function useMobilePanelChangeHandler(
  reviewSource: MobileReviewSource,
  handlePanelChangeAndClearSheet: (panel: MobileSessionPanel) => void,
  handleReviewPanelChange: (panel: MobileSessionPanel) => void,
) {
  return useCallback(
    (panel: MobileSessionPanel) => {
      if (panel === "review" && reviewSource === "github") {
        handlePanelChangeAndClearSheet(panel);
        return;
      }
      handleReviewPanelChange(panel);
    },
    [handlePanelChangeAndClearSheet, handleReviewPanelChange, reviewSource],
  );
}

export function resolveMobileReviewSource(
  hasGitHubPR: boolean,
  hasGitLabMR: boolean,
): MobileReviewSource {
  if (hasGitHubPR) return "github";
  if (hasGitLabMR) return "gitlab";
  return null;
}

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
  selectedFilePreview: boolean;
  selectedDiff: { path: string; content?: string } | null;
  handleOpenFileFromChat: (path: string, repo?: string, preview?: boolean) => void;
  handleClearSelectedDiff: () => void;
  handleOpenFile: (file: OpenFileTab) => void;
  handlePanelChangeAndClearSheet: (panel: MobileSessionPanel) => void;
  topNavHeight: string;
  bottomNavHeight: string;
  reviewSource: MobileReviewSource;
  mrKey?: string;
  prKey?: string;
  reviewPRs: ReturnType<typeof useReviewPRSelection>["prs"];
  selectedReviewPR: ReturnType<typeof useReviewPRSelection>["selectedPR"];
  onSelectReviewPR: ReturnType<typeof useReviewPRSelection>["selectPR"];
};

/** Keeps terminal content's visible bottom glued to the keybar top. When the
 *  keyboard is up, the content area already pads for the bottom nav (which
 *  is now under the keyboard), so we subtract it back out and add the
 *  keyboard height instead. */
export function terminalPaddingBottom(
  keyboardOpen: boolean,
  bottomOffset: number,
  bottomNavHeight: string,
): string {
  return keyboardOpen
    ? `calc(${bottomOffset + KEYBAR_HEIGHT_PX}px - ${bottomNavHeight} - env(safe-area-inset-bottom, 0px))`
    : `${KEYBAR_HEIGHT_PX}px`;
}

export function MobilePanelArea({
  currentMobilePanel,
  activeTaskId,
  isPassthroughMode,
  effectiveSessionId,
  selectedFile,
  selectedFilePreview,
  selectedDiff,
  handleOpenFileFromChat,
  handleClearSelectedDiff,
  handleOpenFile,
  handlePanelChangeAndClearSheet,
  topNavHeight,
  bottomNavHeight,
  reviewSource,
  mrKey,
  prKey,
  reviewPRs,
  selectedReviewPR,
  onSelectReviewPR,
}: MobilePanelAreaProps) {
  const { keyboardOpen, bottomOffset } = useVisualViewportOffset();
  const terminalPadding = terminalPaddingBottom(keyboardOpen, bottomOffset, bottomNavHeight);
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
              key={`${selectedFile.repo ?? ""}\u0000${selectedFile.path}`}
              file={selectedFile}
              sessionId={effectiveSessionId}
              initialMarkdownPreview={selectedFilePreview}
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
          style={{ paddingBottom: terminalPadding }}
        >
          <SessionPanelContent className="p-0 flex-1 min-h-0 flex flex-col">
            <MobileTerminalPane key={effectiveSessionId} sessionId={effectiveSessionId} />
          </SessionPanelContent>
        </div>
      )}
      {currentMobilePanel === "review" && reviewSource === "github" && prKey && (
        <MobileGitHubReviewPanel
          prKey={prKey}
          reviewPRs={reviewPRs}
          selectedReviewPR={selectedReviewPR}
          onSelectReviewPR={onSelectReviewPR}
        />
      )}
      {currentMobilePanel === "review" && reviewSource === "gitlab" && mrKey && (
        <div className="flex min-h-0 flex-1 flex-col" data-testid="mobile-mr-review-panel">
          <MRDetailPanelComponent panelId="mobile-mr-detail" params={{ mrKey }} />
        </div>
      )}
      <MobilePluginPanel currentMobilePanel={currentMobilePanel} />
    </div>
  );
}

/**
 * Renders a mobile-enabled plugin task panel (AC7) — `currentMobilePanel` is
 * a `plugin:<pluginId>:<panelKey>` id from `SessionMobileBottomNav`'s
 * plugin-contributed nav entries. A non-plugin id (the common case) is a
 * no-op, so this stays a single trailing branch instead of one per plugin.
 */
function MobilePluginPanel({ currentMobilePanel }: { currentMobilePanel: MobileSessionPanel }) {
  const parsed = parsePluginPanelId(currentMobilePanel);
  if (!parsed) return null;
  return (
    <div className="flex-1 min-h-0 flex flex-col p-2">
      <PluginTaskPanel
        pluginId={parsed.pluginId}
        panelKey={parsed.panelKey}
        panelId={currentMobilePanel}
        presentation="mobile"
      />
    </div>
  );
}

function MobileGitHubReviewPanel({
  prKey,
  reviewPRs,
  selectedReviewPR,
  onSelectReviewPR,
}: Pick<MobilePanelAreaProps, "prKey" | "reviewPRs" | "selectedReviewPR" | "onSelectReviewPR"> & {
  prKey: string;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="mobile-pr-review-panel">
      <div className="shrink-0 px-2 py-2">
        <ReviewPRSelector
          prs={reviewPRs}
          selectedPR={selectedReviewPR}
          loading={false}
          onSelectPR={onSelectReviewPR}
          className="w-full"
          testIdPrefix="mobile-review-pr-selector"
        />
      </div>
      <div className="min-h-0 flex-1">
        <PRDetailPanelComponent key={prKey} panelId="mobile-pr-detail" params={{ prKey }} />
      </div>
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
  const [selectedFilePreview, setSelectedFilePreview] = useState(false);
  const [trackedSessionId, setTrackedSessionId] = useState<string | null>(effectiveSessionId);
  const latestRequestIdRef = useRef(0);
  const openFileAbortRef = useRef<AbortController | null>(null);

  // Reset viewer when switching sessions — adjust state during render per
  // https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes
  if (trackedSessionId !== effectiveSessionId) {
    setTrackedSessionId(effectiveSessionId);
    setSelectedFile(null);
    setSelectedFilePreview(false);
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
    (path: string, repo?: string, preview = false) => {
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
            setSelectedFilePreview(preview);
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
      setSelectedFilePreview(false);
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
      setSelectedFilePreview(false);
      handlePanelChange(panel);
    },
    [handlePanelChange],
  );

  return {
    selectedFile,
    selectedFilePreview,
    handleOpenFileFromChat,
    handleOpenFile,
    handlePanelChangeAndClearSheet,
  };
}

type SessionMobileFooterProps = {
  sessionId: string | null;
  activePanel: MobileSessionPanel;
  onPanelChange: (panel: MobileSessionPanel) => void;
  planBadge: boolean;
  changesBadge: number;
  hasReview: boolean;
  showStatus: boolean;
  onOpenStatus: () => void;
  connectionIssueSeverity: import("@/lib/types/connection").ConnectionIssueSeverity;
};

function SessionMobileFooter({
  sessionId,
  activePanel,
  onPanelChange,
  planBadge,
  changesBadge,
  hasReview,
  showStatus,
  onOpenStatus,
  connectionIssueSeverity,
}: SessionMobileFooterProps) {
  return (
    <>
      <MobileTerminalKeybar
        sessionId={sessionId}
        visible={activePanel === "terminal"}
        baseBottomOffset={BOTTOM_NAV_HEIGHT}
      />
      <SessionMobileBottomNav
        activePanel={activePanel}
        onPanelChange={onPanelChange}
        planBadge={planBadge}
        changesBadge={changesBadge}
        hasReview={hasReview}
        showStatus={showStatus}
        onOpenStatus={onOpenStatus}
        connectionIssueSeverity={connectionIssueSeverity}
      />
    </>
  );
}

function StatusAwareSessionMobileFooter(
  props: Omit<SessionMobileFooterProps, "showStatus" | "onOpenStatus" | "connectionIssueSeverity">,
) {
  const { enabled, issueSeverity, openStatusDrawer } = useAppStatusDrawer();
  return (
    <SessionMobileFooter
      {...props}
      showStatus={enabled}
      onOpenStatus={openStatusDrawer}
      connectionIssueSeverity={issueSeverity}
    />
  );
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
  const {
    selectedFile,
    selectedFilePreview,
    handleOpenFileFromChat,
    handleOpenFile,
    handlePanelChangeAndClearSheet,
  } = useMobilePanelHandlers({ effectiveSessionId, handlePanelChange });
  const mobilePR = useReviewPRSelection(activeTaskId);
  const mobileMR = useMobileMRSelection(
    activeTaskId,
    effectiveSessionId,
    currentMobilePanel,
    handlePanelChangeAndClearSheet,
    mobilePR.prs.length > 0,
  );
  const reviewSource = resolveMobileReviewSource(mobilePR.prs.length > 0, mobileMR.mrs.length > 0);
  const effectiveMobilePanel = useEffectiveMobilePanel(
    currentMobilePanel,
    reviewSource,
    handlePanelChange,
  );
  const handleMobilePanelChange = useMobilePanelChangeHandler(
    reviewSource,
    handlePanelChangeAndClearSheet,
    mobileMR.handlePanelChange,
  );
  return (
    <div className="h-dvh relative bg-background" data-testid="mobile-task-layout">
      <MobileTopBarSticky
        {...props}
        activeTaskId={activeTaskId}
        effectiveSessionId={effectiveSessionId}
        onMenuClick={handleMenuClick}
        showApproveButton={showApproveButton}
        onApprove={handleApprove}
      />
      <MobilePanelArea
        currentMobilePanel={effectiveMobilePanel}
        activeTaskId={activeTaskId}
        isPassthroughMode={isPassthroughMode}
        effectiveSessionId={effectiveSessionId}
        selectedFile={selectedFile}
        selectedFilePreview={selectedFilePreview}
        selectedDiff={selectedDiff}
        handleOpenFileFromChat={handleOpenFileFromChat}
        handleClearSelectedDiff={handleClearSelectedDiff}
        handleOpenFile={handleOpenFile}
        handlePanelChangeAndClearSheet={handlePanelChangeAndClearSheet}
        topNavHeight={TOP_NAV_HEIGHT}
        bottomNavHeight={BOTTOM_NAV_HEIGHT}
        reviewSource={reviewSource}
        mrKey={mobileMR.selectedMR ? mrTaskKey(mobileMR.selectedMR) : undefined}
        prKey={mobilePR.selectedPR ? prTaskKey(mobilePR.selectedPR) : undefined}
        reviewPRs={mobilePR.prs}
        selectedReviewPR={mobilePR.selectedPR}
        onSelectReviewPR={mobilePR.selectPR}
      />

      <StatusAwareSessionMobileFooter
        sessionId={effectiveSessionId ?? null}
        activePanel={effectiveMobilePanel}
        onPanelChange={handleMobilePanelChange}
        planBadge={hasUnseenPlanUpdate}
        changesBadge={totalChangesCount}
        hasReview={reviewSource !== null}
      />

      {/* Task Switcher Sheet */}
      <SessionTaskSwitcherSheet
        open={isTaskSwitcherOpen}
        onOpenChange={setMobileSessionTaskSwitcherOpen}
        workspaceId={props.workspaceId}
        workflowId={props.workflowId}
        presentation="drawer"
      />

      <SessionMobileReviewDialog
        sessionId={effectiveSessionId}
        taskId={activeTaskId}
        onOpenFile={handleOpenFileFromChat}
      />
    </div>
  );
});
