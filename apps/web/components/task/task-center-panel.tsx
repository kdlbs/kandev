"use client";

import { memo, useCallback, useState, useEffect, useMemo } from "react";
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
import type { OpenFileTab, FileContentResponse } from "@/lib/types/backend";
import { useAppStore } from "@/components/state-provider";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { getWebSocketClient } from "@/lib/ws/connection";
import { executeApprove } from "@/lib/services/session-approve";
import { requestFileContent } from "@/lib/ws/workspace-files";
import {
  setOpenFileTabs as saveOpenFileTabs,
  getActiveTabForSession,
  setActiveTabForSession,
} from "@/lib/local-storage";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { calculateHash } from "@/lib/utils/file-diff";
import { useToast } from "@/components/toast-provider";
import { useFileTabRestoration, useFileSaveDelete } from "./task-center-panel-restoration";
import { useNormalizedTaskReviews } from "./review-panel-provider";
import { useReviewItemSelection } from "./review-selection";
import { getFileTabKey, upsertOpenFileTab } from "./task-center-panel-file-tabs";
import { TaskCenterReviewContent } from "./task-center-review-content";
import { defaultMarkdownFileMode, type MarkdownFileMode } from "./markdown-file-mode";

import type { SelectedDiff } from "./task-layout";
import { useTranslation } from "react-i18next";

type TaskCenterPanelProps = {
  selectedDiff: SelectedDiff | null;
  openFileRequest: OpenFileTab | null;
  onDiffHandled: () => void;
  onFileOpenHandled: () => void;
  onActiveFileChange?: (filePath: string | null) => void;
  sessionId?: string | null;
};

function useSessionApprove(activeSessionId: string | null, activeTaskId: string | null) {
  const activeSession = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const isAgentWorking = activeSession?.state === "STARTING" || activeSession?.state === "RUNNING";
  const isPassthroughMode = useMemo(() => isPassthroughSession(activeSession), [activeSession]);
  const showApproveButton =
    !!activeSession?.review_status && activeSession.review_status !== "approved" && !isAgentWorking;
  const handleApprove = useCallback(async () => {
    if (!activeSessionId || !activeTaskId) return;
    try {
      await executeApprove(activeSessionId, activeTaskId, setTaskSession);
    } catch (error) {
      console.error("Failed to approve session:", error);
    }
  }, [activeSessionId, activeTaskId, setTaskSession]);
  return { activeSession, isPassthroughMode, showApproveButton, handleApprove };
}

function useLeftTabState(
  activeSessionId: string | null,
  hasChanges: boolean | undefined,
  onActiveFileChange?: (filePath: string | null) => void,
) {
  const [leftTab, setLeftTab] = useState(() => {
    if (typeof window !== "undefined" && activeSessionId) {
      const savedTab = getActiveTabForSession(activeSessionId, "chat");
      if (savedTab === "chat" || savedTab === "changes") return savedTab;
    }
    return "chat";
  });
  const [showRequestChangesTooltip, setShowRequestChangesTooltip] = useState(false);

  useEffect(() => {
    if (leftTab === "changes" && !hasChanges) queueMicrotask(() => setLeftTab("chat"));
  }, [leftTab, hasChanges]);
  useEffect(() => {
    const handler = () => {
      if (hasChanges) setLeftTab("changes");
    };
    window.addEventListener("switch-to-changes-tab", handler);
    return () => window.removeEventListener("switch-to-changes-tab", handler);
  }, [hasChanges]);
  useEffect(() => {
    if (leftTab.startsWith("file:")) onActiveFileChange?.(leftTab.replace("file:", ""));
    else onActiveFileChange?.(null);
  }, [leftTab, onActiveFileChange]);

  const handleTabChange = useCallback(
    (tab: string) => {
      setLeftTab(tab);
      if (activeSessionId) setActiveTabForSession(activeSessionId, tab);
    },
    [activeSessionId],
  );
  const handleRequestChanges = useCallback(() => {
    setLeftTab("chat");
    setShowRequestChangesTooltip(true);
    setTimeout(() => setShowRequestChangesTooltip(false), 5000);
  }, []);
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
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
  setSavingFiles: React.Dispatch<React.SetStateAction<Set<string>>>;
  setLeftTab: (tab: string) => void;
  handleTabChange: (tab: string) => void;
  leftTab: string;
};

function useFileTabOperations({
  activeSessionId,
  openFileTabs,
  setOpenFileTabs,
  setSavingFiles,
  setLeftTab,
  handleTabChange,
  leftTab,
}: FileTabOperationsOptions) {
  const { t } = useTranslation();
  const { toast } = useToast();

  const addFileTab = useCallback(
    (fileTab: OpenFileTab) => {
      setOpenFileTabs((prev) => upsertOpenFileTab(prev, fileTab));
      setLeftTab(`file:${getFileTabKey(fileTab)}`);
    },
    [setOpenFileTabs, setLeftTab],
  );

  const handleOpenFileFromChat = useCallback(
    async (filePath: string, repo?: string) => {
      const client = getWebSocketClient();
      if (!client || !activeSessionId) return;
      try {
        const response: FileContentResponse = await requestFileContent(
          client,
          activeSessionId,
          filePath,
          repo,
        );
        const fileName = filePath.split("/").pop() || filePath;
        const hash = await calculateHash(response.content);
        addFileTab({
          path: filePath,
          repo,
          name: fileName,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
          markdownMode: defaultMarkdownFileMode(filePath),
        });
      } catch (error) {
        toast({
          title: t("task:failedToOpenFile"),
          description: error instanceof Error ? error.message : t("task:unknownError"),
          variant: "error",
        });
      }
    },
    [activeSessionId, toast, addFileTab],
  );

  const handleCloseFileTab = useCallback(
    (fileKey: string) => {
      setOpenFileTabs((prev) => prev.filter((tab) => getFileTabKey(tab) !== fileKey));
      if (leftTab === `file:${fileKey}`) handleTabChange("chat");
    },
    [leftTab, handleTabChange, setOpenFileTabs],
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

  const handleMarkdownModeChange = useCallback(
    (fileKey: string, markdownMode: MarkdownFileMode) => {
      setOpenFileTabs((prev) =>
        prev.map((tab) => (getFileTabKey(tab) === fileKey ? { ...tab, markdownMode } : tab)),
      );
    },
    [setOpenFileTabs],
  );

  const { handleFileSave, handleFileDelete } = useFileSaveDelete({
    activeSessionId,
    openFileTabs,
    setOpenFileTabs,
    setSavingFiles,
    handleCloseFileTab,
  });

  return {
    handleOpenFileFromChat,
    handleCloseFileTab,
    handleFileChange,
    handleMarkdownModeChange,
    handleFileSave,
    handleFileDelete,
    addFileTab,
  };
}

function useCenterPanelTabs(
  openFileTabs: OpenFileTab[],
  handleCloseFileTab: (fileKey: string) => void,
  hasChanges: boolean | undefined,
  reviewLabel: string | null,
) {
  const { t } = useTranslation();
  const tabs: SessionTab[] = useMemo(() => {
    const staticTabs: SessionTab[] = [
      ...(hasChanges ? [{ id: "changes", label: t("task:allChanges") }] : []),
      { id: "chat", label: t("task:chat") },
      ...(reviewLabel ? [{ id: "pr", label: reviewLabel }] : []),
    ];
    const fileTabs: SessionTab[] = openFileTabs.map((tab) => ({
      id: `file:${getFileTabKey(tab)}`,
      label: tab.isDirty ? `${tab.name} *` : tab.name,
      icon: tab.isDirty ? <span className="h-2 w-2 rounded-full bg-yellow-500" /> : undefined,
      closable: true,
      onClose: (e: React.MouseEvent) => {
        e.stopPropagation();
        handleCloseFileTab(getFileTabKey(tab));
      },
      className: "cursor-pointer group gap-1.5 data-[state=active]:bg-muted",
    }));
    return [...staticTabs, ...fileTabs];
  }, [openFileTabs, handleCloseFileTab, hasChanges, reviewLabel, t]);
  const separatorAfterIndex = useMemo(() => {
    if (openFileTabs.length === 0) return undefined;
    const staticCount = (hasChanges ? 1 : 0) + 1 + (reviewLabel ? 1 : 0);
    return staticCount - 1;
  }, [openFileTabs.length, hasChanges, reviewLabel]);
  return { tabs, separatorAfterIndex };
}

function useTaskReview(taskId: string | null) {
  const { t } = useTranslation();
  const reviews = useNormalizedTaskReviews(taskId);
  const { selectedReview, selectReview } = useReviewItemSelection(taskId, reviews);
  return {
    reviews,
    selectedReview,
    selectReview,
    reviewLabel: reviewLabelForReviews(reviews, t),
  };
}

function reviewLabelForReviews(
  reviews: readonly { providerId: string }[],
  t: (key: string, options?: { count: number }) => string,
): string | null {
  if (reviews.length === 0) return null;
  if (reviews.length > 1) return t("task:reviewsCount", { count: reviews.length });
  const providerId = reviews[0]?.providerId;
  if (providerId === "github") return t("task:pullRequest2");
  if (providerId === "gitlab") return t("task:mergeRequestLabel");
  return t("task:review");
}

function usePersistOpenFileTabs(activeSessionId: string | null, openFileTabs: OpenFileTab[]) {
  useEffect(() => {
    if (!activeSessionId) return;
    saveOpenFileTabs(
      activeSessionId,
      openFileTabs.map(({ path, name, repo, markdownMode }) => ({
        path,
        name,
        repo,
        ...(markdownMode ? { markdownMode } : {}),
      })),
    );
  }, [activeSessionId, openFileTabs]);
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
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId);
  const hasChanges = useMemo(() => {
    const hasUncommittedChanges = gitStatus?.files && Object.keys(gitStatus.files).length > 0;
    return (hasUncommittedChanges || (commits && commits.length > 0)) as boolean;
  }, [gitStatus, commits]);
  const { activeSession, isPassthroughMode, showApproveButton, handleApprove } = useSessionApprove(
    activeSessionId,
    activeTaskId,
  );
  const { reviews, selectedReview, selectReview, reviewLabel } = useTaskReview(activeTaskId);
  const [openFileTabs, setOpenFileTabs] = useState<OpenFileTab[]>([]);
  const [savingFiles, setSavingFiles] = useState<Set<string>>(new Set());
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);
  const {
    leftTab,
    setLeftTab,
    showRequestChangesTooltip,
    setShowRequestChangesTooltip,
    handleTabChange,
    handleRequestChanges,
  } = useLeftTabState(activeSessionId, hasChanges, onActiveFileChange);
  useFileTabRestoration({ activeSessionId, leftTab, setLeftTab, setOpenFileTabs });
  usePersistOpenFileTabs(activeSessionId, openFileTabs);
  const fileTabOps = useFileTabOperations({
    activeSessionId,
    openFileTabs,
    setOpenFileTabs,
    setSavingFiles,
    setLeftTab,
    handleTabChange,
    leftTab,
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

  useEffect(() => {
    if (!openFileRequest) return;
    queueMicrotask(async () => {
      const hash = openFileRequest.originalHash || (await calculateHash(openFileRequest.content));
      fileTabOps.addFileTab({
        ...openFileRequest,
        originalContent: openFileRequest.originalContent || openFileRequest.content,
        originalHash: hash,
        isDirty: openFileRequest.isDirty ?? false,
      });
      onFileOpenHandled();
    });
  }, [openFileRequest, onFileOpenHandled, fileTabOps.addFileTab]); // eslint-disable-line react-hooks/exhaustive-deps

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
  const {
    handleOpenFileFromChat,
    handleFileChange,
    handleMarkdownModeChange,
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
            onOpenFile={handleOpenFileFromChat}
            onMarkdownModeChange={(mode) => handleMarkdownModeChange(getFileTabKey(tab), mode)}
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
}: {
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  sessionId: string | null | undefined;
  taskId: string | null;
  showRequestChangesTooltip: boolean;
  onDismissTooltip: () => void;
  onOpenFile: (filePath: string) => void;
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
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onDismissTooltip}
        onOpenFileAtLine={onOpenFile}
      />
    </TabsContent>
  );
}
