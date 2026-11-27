"use client";

import { memo, useCallback, useState, useEffect, type RefObject } from "react";
import { IconCheck, IconChevronDown, IconX } from "@tabler/icons-react";
import { TabsContent } from "@kandev/ui/tabs";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { SessionPanel } from "@kandev/ui/pannel-session";
import { TaskChatPanel } from "./task-chat-panel";
import { TaskChangesPanel } from "./task-changes-panel";
import { FileTabContent } from "./file-tab-content";
import { PassthroughToolbar } from "./passthrough-toolbar";
import { useTabletFileState } from "./use-tablet-file-state";
import type { RenameAdapter } from "@/hooks/file-rename-continuity";
import type { OpenFileTab } from "@/lib/types/backend";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { isRenameReserved, treePath } from "@/hooks/file-rename-continuity";
import { SessionTabs } from "@/components/session-tabs";
import { useTabletSessionTab } from "@/hooks/domains/session/use-tablet-session-tab";
import { useToast } from "@/components/toast-provider";
import { useFileSaveDelete } from "./task-center-panel-restoration";
import {
  getFileTabKey,
  upsertOpenFileTab,
  type FileEditorTab,
} from "./task-center-panel-file-tabs";
import { TaskCenterReviewContent } from "./task-center-review-content";
import { useTaskCenterFileOpen } from "@/hooks/use-task-center-file-open";
import { tabletSelectionOwner, hasPendingTabletFileOpen } from "@/lib/state/tablet-file-authority";
import { usePendingTabletFileOpen } from "@/hooks/domains/session/use-tablet-file-admission";
import {
  useCenterPanelGitChanges,
  useCenterPanelTabs,
  useLiveFileTabs,
  useSessionApprove,
  useTabletFileRequest,
  useTaskReview,
} from "./task-center-panel-state";

import type { SelectedDiff } from "./task-layout";
import type { HistoricalTurnDiffTarget } from "@/lib/state/diff-target-types";
import { useTranslation } from "react-i18next";

type TaskCenterPanelProps = {
  selectedDiff: SelectedDiff | null;
  openFileRequest: OpenFileTab | null;
  onDiffHandled: () => void;
  onFileOpenHandled: () => void;
  onActiveFileChange?: (filePath: string | null) => void;
  sessionId?: string | null;
  renameAdapter?: RefObject<RenameAdapter | null>;
};

function useLeftTabState(
  activeSessionId: string | null,
  hasChanges: boolean | undefined,
  onActiveFileChange?: (filePath: string | null) => void,
  hasReviews?: boolean,
  reviewsLoading?: boolean,
) {
  const [leftTab, setLeftTab] = useTabletSessionTab(activeSessionId);
  const store = useAppStoreApi();
  const [showRequestChangesTooltip, setShowRequestChangesTooltip] = useState(false);
  const pendingFile = usePendingTabletFileOpen(activeSessionId);

  useEffect(() => {
    if (leftTab !== "changes" || hasChanges !== false || !activeSessionId || pendingFile) return;
    let current = true;
    const owner = tabletSelectionOwner(store, activeSessionId);
    queueMicrotask(() => {
      if (
        current &&
        !hasPendingTabletFileOpen(store, activeSessionId) &&
        store.getState().tabletSession.activeTabBySessionId[activeSessionId] === "changes" &&
        tabletSelectionOwner(store, activeSessionId) === owner
      )
        setLeftTab("chat");
    });
    return () => {
      current = false;
    };
  }, [leftTab, hasChanges, setLeftTab, store, activeSessionId, pendingFile]);
  useEffect(() => {
    if (
      leftTab !== "pr" ||
      reviewsLoading ||
      hasReviews !== false ||
      !activeSessionId ||
      pendingFile
    )
      return;
    let current = true;
    const owner = tabletSelectionOwner(store, activeSessionId);
    queueMicrotask(() => {
      if (
        current &&
        !hasPendingTabletFileOpen(store, activeSessionId) &&
        store.getState().tabletSession.activeTabBySessionId[activeSessionId] === "pr" &&
        tabletSelectionOwner(store, activeSessionId) === owner
      )
        setLeftTab("chat");
    });
    return () => {
      current = false;
    };
  }, [leftTab, hasReviews, reviewsLoading, setLeftTab, store, activeSessionId, pendingFile]);
  useEffect(() => {
    const handler = () => {
      if (hasChanges) setLeftTab("changes");
    };
    window.addEventListener("switch-to-changes-tab", handler);
    return () => window.removeEventListener("switch-to-changes-tab", handler);
  }, [hasChanges, setLeftTab]);
  useEffect(() => {
    const handler = () => setLeftTab("files");
    window.addEventListener("switch-to-files-tab", handler);
    return () => window.removeEventListener("switch-to-files-tab", handler);
  }, [setLeftTab]);
  useEffect(() => {
    if (leftTab.startsWith("file:")) onActiveFileChange?.(leftTab.replace("file:", ""));
    else onActiveFileChange?.(null);
  }, [leftTab, onActiveFileChange]);

  const handleTabChange = setLeftTab;
  const handleRequestChanges = useCallback(() => {
    setLeftTab("chat");
    setShowRequestChangesTooltip(true);
    setTimeout(() => setShowRequestChangesTooltip(false), 5000);
  }, [setLeftTab]);
  return {
    leftTab,
    setLeftTab,
    showRequestChangesTooltip,
    setShowRequestChangesTooltip,
    handleTabChange,
    handleRequestChanges,
  };
}

type FileTabOperationsOptions = {
  activeSessionId: string | null;
  openFileTabs: OpenFileTab[];
  fileTabsRef: RefObject<OpenFileTab[]>;
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
  setLeftTab: (tab: string) => void;
  handleTabChange: (tab: string) => void;
  leftTab: string;
  retireFile: (file: OpenFileTab) => void;
};

function useFileTabOperations({
  activeSessionId,
  openFileTabs,
  fileTabsRef,
  setOpenFileTabs,
  setLeftTab,
  handleTabChange,
  leftTab,
  retireFile,
}: FileTabOperationsOptions) {
  const { toast } = useToast();
  const { t } = useTranslation();
  const store = useAppStoreApi();

  const addFileTab = useCallback(
    (fileTab: OpenFileTab) => {
      if (activeSessionId && isRenameReserved(store, activeSessionId, fileTab.path, fileTab.repo)) {
        toast({
          title: t("task:failedToOpenFile"),
          variant: "error",
        });
        return;
      }
      const requestedPath = treePath(fileTab);
      const next = upsertOpenFileTab(fileTabsRef.current, fileTab);
      setOpenFileTabs(next);
      const selected = next.find(
        (tab) =>
          getFileTabKey(tab) === getFileTabKey(fileTab) ||
          (requestedPath !== null && treePath(tab) === requestedPath),
      );
      if (selected) setLeftTab(`file:${getFileTabKey(selected)}`);
    },
    [activeSessionId, store, t, toast, fileTabsRef, setOpenFileTabs, setLeftTab],
  );

  const handleOpenFileFromChat = useTaskCenterFileOpen({
    activeSessionId,
    addFileTab,
    toast,
  });

  const handleCloseFileTab = useCallback(
    (fileKey: string, instanceId?: symbol) => {
      const closing = openFileTabs.find(
        (file: FileEditorTab) =>
          getFileTabKey(file) === fileKey &&
          (instanceId === undefined || file.instanceId === instanceId),
      );
      if (closing) retireFile(closing);
      setOpenFileTabs((prev) =>
        prev.filter(
          (tab: FileEditorTab) =>
            getFileTabKey(tab) !== fileKey ||
            (instanceId !== undefined && tab.instanceId !== instanceId),
        ),
      );
      if (leftTab === `file:${fileKey}`) handleTabChange("chat");
    },
    [leftTab, handleTabChange, setOpenFileTabs, openFileTabs, retireFile],
  );

  const handleFileChange = useCallback(
    (path: string, newContent: string, repo?: string) => {
      const fileKey = getFileTabKey({ path, repo });
      setOpenFileTabs((prev) =>
        prev.map((tab) =>
          getFileTabKey(tab) === fileKey
            ? { ...tab, content: newContent, isDirty: newContent !== tab.originalContent }
            : tab,
        ),
      );
    },
    [setOpenFileTabs],
  );

  const handleRenderedPreviewToggle = useCallback(
    (fileKey: string) => {
      setOpenFileTabs((prev) =>
        prev.map((tab) =>
          getFileTabKey(tab) === fileKey ? { ...tab, renderedPreview: !tab.renderedPreview } : tab,
        ),
      );
    },
    [setOpenFileTabs],
  );

  const { handleFileSave, handleFileDelete, savingFiles } = useFileSaveDelete({
    activeSessionId,
    openFileTabs,
    setOpenFileTabs,
    handleCloseFileTab,
  });

  return {
    handleOpenFileFromChat,
    handleCloseFileTab,
    handleFileChange,
    handleRenderedPreviewToggle,
    handleFileSave,
    handleFileDelete,
    savingFiles,
    addFileTab,
  };
}

function useCenterPanelState(props: TaskCenterPanelProps) {
  const {
    selectedDiff: externalSelectedDiff,
    openFileRequest,
    onDiffHandled,
    onFileOpenHandled,
    onActiveFileChange,
  } = props;
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const hasChanges = useCenterPanelGitChanges(activeSessionId);
  const { activeSession, isPassthroughMode, showApproveButton, handleApprove } = useSessionApprove(
    activeSessionId,
    activeTaskId,
  );
  const { reviews, reviewsLoading, selectedReview, selectReview, reviewLabel } =
    useTaskReview(activeTaskId);
  const [openFileTabs, setOpenFileTabs, fileTabsRef] = useLiveFileTabs();
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);
  const {
    leftTab,
    setLeftTab,
    showRequestChangesTooltip,
    setShowRequestChangesTooltip,
    handleTabChange,
    handleRequestChanges,
  } = useLeftTabState(
    activeSessionId,
    hasChanges,
    onActiveFileChange,
    reviews.length > 0,
    reviewsLoading,
  );
  const { retireFile } = useTabletFileState({
    adapter: props.renameAdapter,
    session: activeSessionId,
    tabs: openFileTabs,
    setTabs: setOpenFileTabs,
    tabsRef: fileTabsRef,
    selected: leftTab,
    setSelected: setLeftTab,
  });
  const fileTabOps = useFileTabOperations({
    activeSessionId,
    openFileTabs,
    fileTabsRef,
    setOpenFileTabs,
    setLeftTab,
    handleTabChange,
    leftTab,
    retireFile,
  });
  const { tabs, separatorAfterIndex } = useCenterPanelTabs(
    openFileTabs,
    fileTabOps.handleCloseFileTab,
    hasChanges,
    reviewLabel,
  );

  useEffect(() => {
    if (externalSelectedDiff) {
      queueMicrotask(() => {
        setSelectedDiff(externalSelectedDiff);
        setLeftTab("changes");
        onDiffHandled();
      });
    }
  }, [externalSelectedDiff, onDiffHandled, setLeftTab]);

  useTabletFileRequest(openFileRequest, onFileOpenHandled, fileTabOps.addFileTab);

  return {
    activeTaskId,
    activeSessionId,
    activeSession,
    isPassthroughMode,
    showApproveButton,
    handleApprove,
    reviews,
    selectedReview,
    selectReview,
    openFileTabs,
    savingFiles: fileTabOps.savingFiles,
    selectedDiff,
    setSelectedDiff,
    leftTab,
    showRequestChangesTooltip,
    setShowRequestChangesTooltip,
    handleTabChange,
    handleRequestChanges,
    fileTabOps,
    tabs,
    separatorAfterIndex,
  };
}

export const TaskCenterPanel = memo(function TaskCenterPanel(props: TaskCenterPanelProps) {
  const { sessionId = null } = props;
  const state = useCenterPanelState(props);
  const {
    activeTaskId,
    activeSessionId,
    activeSession,
    isPassthroughMode,
    showApproveButton,
    handleApprove,
    reviews,
    selectedReview,
    selectReview,
    openFileTabs,
    savingFiles,
    selectedDiff,
    setSelectedDiff,
    leftTab,
    showRequestChangesTooltip,
    setShowRequestChangesTooltip,
    handleTabChange,
    handleRequestChanges,
    fileTabOps,
    tabs,
    separatorAfterIndex,
  } = state;
  const handleOpenHistoricalDiff = useCallback(
    (target: HistoricalTurnDiffTarget) => {
      setSelectedDiff({ path: target.path ?? "", historical: target });
      handleTabChange("changes");
    },
    [handleTabChange, setSelectedDiff],
  );
  const {
    handleOpenFileFromChat,
    handleFileChange,
    handleRenderedPreviewToggle,
    handleFileSave,
    handleFileDelete,
  } = fileTabOps;

  const approveContent = showApproveButton ? (
    <ApproveButtonGroup onApprove={handleApprove} onRequestChanges={handleRequestChanges} />
  ) : undefined;

  return (
    <SessionPanel borderSide="right" margin="right">
      <SessionTabs
        tabs={tabs}
        activeTab={leftTab}
        onTabChange={handleTabChange}
        separatorAfterIndex={separatorAfterIndex}
        className="flex-1 min-h-0 flex flex-col gap-2"
        rightContent={approveContent}
      >
        <TabsContent value="changes" className="flex-1 min-h-0">
          <TaskChangesPanel
            selectedDiff={selectedDiff}
            onClearSelected={() => setSelectedDiff(null)}
            onOpenFile={handleOpenFileFromChat}
          />
        </TabsContent>
        <ChatTabContent
          activeTaskId={activeTaskId}
          isPassthroughMode={isPassthroughMode}
          sessionId={sessionId}
          taskId={sessionId ? activeTaskId : null}
          showRequestChangesTooltip={showRequestChangesTooltip}
          onDismissTooltip={() => setShowRequestChangesTooltip(false)}
          onOpenFile={handleOpenFileFromChat}
          onOpenHistoricalDiff={handleOpenHistoricalDiff}
        />
        {reviews.length > 0 && activeSessionId && (
          <TaskCenterReviewContent
            reviews={reviews}
            selectedReview={selectedReview}
            onSelectReview={selectReview}
          />
        )}
        {openFileTabs.map((tab) => (
          <FileTabContent
            key={getFileTabKey(tab)}
            tab={tab}
            activeSession={activeSession}
            activeSessionId={activeSessionId}
            taskId={activeSessionId ? activeTaskId : null}
            isSaving={savingFiles.has(getFileTabKey(tab))}
            onFileChange={handleFileChange}
            onFileSave={handleFileSave}
            onFileDelete={handleFileDelete}
            onTogglePreview={() => handleRenderedPreviewToggle(getFileTabKey(tab))}
          />
        ))}
      </SessionTabs>
    </SessionPanel>
  );
});

function ApproveButtonGroup({
  onApprove,
  onRequestChanges,
}: {
  onApprove: () => void;
  onRequestChanges: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-0.5">
      <Button
        type="button"
        size="sm"
        className="h-6 gap-1.5 px-2.5 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-medium rounded-r-none border-r border-emerald-700/30"
        onClick={onApprove}
      >
        <IconCheck className="h-3.5 w-3.5" />
        {t("task:approve")}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            size="sm"
            className="h-6 w-6 p-0 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white rounded-l-none"
          >
            <IconChevronDown className="h-3 w-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuItem onClick={onApprove} className="cursor-pointer">
            <IconCheck className="h-4 w-4 mr-2" />
            {t("task:approveAndContinue")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={onRequestChanges}
            className="cursor-pointer text-amber-600 dark:text-amber-500"
          >
            <IconX className="h-4 w-4 mr-2" />
            {t("task:requestChanges")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function ChatTabContent({
  activeTaskId,
  isPassthroughMode,
  sessionId,
  taskId,
  showRequestChangesTooltip,
  onDismissTooltip,
  onOpenFile,
  onOpenHistoricalDiff,
}: {
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  sessionId: string | null | undefined;
  taskId: string | null;
  showRequestChangesTooltip: boolean;
  onDismissTooltip: () => void;
  onOpenFile: (filePath: string, repositoryName?: string) => void;
  onOpenHistoricalDiff: (target: HistoricalTurnDiffTarget) => void;
}) {
  const { t } = useTranslation();
  if (!activeTaskId) {
    return (
      <TabsContent
        value="chat"
        className="flex flex-col min-h-0 flex-1"
        style={{ minHeight: "200px" }}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          {t("task:noTaskSelected")}
        </div>
      </TabsContent>
    );
  }
  if (isPassthroughMode) {
    return (
      <TabsContent
        value="chat"
        className="flex flex-col min-h-0 flex-1"
        style={{ minHeight: "200px" }}
      >
        <div className="flex-1 min-h-0 h-full" style={{ minHeight: "150px" }}>
          <PassthroughToolbar key={activeTaskId} sessionId={sessionId} taskId={activeTaskId} />
        </div>
      </TabsContent>
    );
  }
  return (
    <TabsContent
      value="chat"
      className="flex flex-col min-h-0 flex-1"
      style={{ minHeight: "200px" }}
    >
      <TaskChatPanel
        sessionId={sessionId}
        taskId={taskId}
        onOpenFile={onOpenFile}
        onOpenHistoricalDiff={onOpenHistoricalDiff}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onDismissTooltip}
        onOpenFileAtLine={onOpenFile}
      />
    </TabsContent>
  );
}
