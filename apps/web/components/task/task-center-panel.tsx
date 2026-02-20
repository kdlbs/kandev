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
import { FileEditorContent } from "./file-editor-content";
import { FileImageViewer } from "./file-image-viewer";
import { FileBinaryViewer } from "./file-binary-viewer";
import { PassthroughTerminal } from "./passthrough-terminal";
import type { OpenFileTab, FileContentResponse } from "@/lib/types/backend";
import { useAppStore } from "@/components/state-provider";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { approveSessionAction } from "@/app/actions/workspaces";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import {
  setOpenFileTabs as saveOpenFileTabs,
  getActiveTabForSession,
  setActiveTabForSession,
} from "@/lib/local-storage";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { calculateHash } from "@/lib/utils/file-diff";
import { getFileCategory } from "@/lib/utils/file-types";
import { useToast } from "@/components/toast-provider";
import { useFileTabRestoration, useFileSaveDelete } from "./task-center-panel-restoration";

import type { SelectedDiff } from "./task-layout";

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
  const isPassthroughMode = useMemo(() => {
    if (!activeSession?.agent_profile_snapshot) return false;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const snapshot = activeSession.agent_profile_snapshot as any;
    return snapshot?.cli_passthrough === true;
  }, [activeSession?.agent_profile_snapshot]);
  const showApproveButton =
    !!activeSession?.review_status && activeSession.review_status !== "approved" && !isAgentWorking;
  const handleApprove = useCallback(async () => {
    if (!activeSessionId || !activeTaskId) return;
    try {
      const response = await approveSessionAction(activeSessionId);
      if (response?.session) setTaskSession(response.session);
      if (
        response?.workflow_step?.events?.on_enter?.some(
          (a: { type: string }) => a.type === "auto_start_agent",
        )
      ) {
        const client = getWebSocketClient();
        if (client)
          client.send({
            type: "request",
            action: "orchestrator.start",
            payload: {
              task_id: activeTaskId,
              session_id: activeSessionId,
              workflow_step_id: response.workflow_step.id,
            },
          });
      }
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
  const { toast } = useToast();

  const addFileTab = useCallback(
    (fileTab: OpenFileTab) => {
      setOpenFileTabs((prev) => {
        if (prev.some((t) => t.path === fileTab.path)) return prev;
        const maxTabs = 4;
        return prev.length >= maxTabs ? [...prev.slice(1), fileTab] : [...prev, fileTab];
      });
      setLeftTab(`file:${fileTab.path}`);
    },
    [setOpenFileTabs, setLeftTab],
  );

  const handleOpenFileFromChat = useCallback(
    async (filePath: string) => {
      const client = getWebSocketClient();
      if (!client || !activeSessionId) return;
      try {
        const response: FileContentResponse = await requestFileContent(
          client,
          activeSessionId,
          filePath,
        );
        const fileName = filePath.split("/").pop() || filePath;
        const hash = await calculateHash(response.content);
        addFileTab({
          path: filePath,
          name: fileName,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
        });
      } catch (error) {
        toast({
          title: "Failed to open file",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [activeSessionId, toast, addFileTab],
  );

  const handleCloseFileTab = useCallback(
    (path: string) => {
      setOpenFileTabs((prev) => prev.filter((t) => t.path !== path));
      if (leftTab === `file:${path}`) handleTabChange("chat");
    },
    [leftTab, handleTabChange, setOpenFileTabs],
  );

  const handleFileChange = useCallback(
    (path: string, newContent: string) => {
      setOpenFileTabs((prev) =>
        prev.map((tab) =>
          tab.path === path
            ? { ...tab, content: newContent, isDirty: newContent !== tab.originalContent }
            : tab,
        ),
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
    handleFileSave,
    handleFileDelete,
    addFileTab,
  };
}

function useCenterPanelTabs(
  openFileTabs: OpenFileTab[],
  handleCloseFileTab: (path: string) => void,
  hasChanges: boolean | undefined,
) {
  const tabs: SessionTab[] = useMemo(() => {
    const staticTabs: SessionTab[] = [
      ...(hasChanges ? [{ id: "changes", label: "All changes" }] : []),
      { id: "chat", label: "Chat" },
    ];
    const fileTabs: SessionTab[] = openFileTabs.map((tab) => ({
      id: `file:${tab.path}`,
      label: tab.isDirty ? `${tab.name} *` : tab.name,
      icon: tab.isDirty ? <span className="h-2 w-2 rounded-full bg-yellow-500" /> : undefined,
      closable: true,
      onClose: (e: React.MouseEvent) => {
        e.stopPropagation();
        handleCloseFileTab(tab.path);
      },
      className: "cursor-pointer group gap-1.5 data-[state=active]:bg-muted",
    }));
    return [...staticTabs, ...fileTabs];
  }, [openFileTabs, handleCloseFileTab, hasChanges]);
  const separatorAfterIndex = useMemo(() => {
    if (openFileTabs.length === 0) return undefined;
    return hasChanges ? 1 : 0;
  }, [openFileTabs.length, hasChanges]);
  return { tabs, separatorAfterIndex };
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
  useEffect(() => {
    if (!activeSessionId) return;
    saveOpenFileTabs(
      activeSessionId,
      openFileTabs.map(({ path, name }) => ({ path, name })),
    );
  }, [activeSessionId, openFileTabs]);
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
  const { handleOpenFileFromChat, handleFileChange, handleFileSave, handleFileDelete } = fileTabOps;

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
          activeSessionId={activeSessionId}
          sessionId={sessionId}
          showRequestChangesTooltip={showRequestChangesTooltip}
          onDismissTooltip={() => setShowRequestChangesTooltip(false)}
          onOpenFile={handleOpenFileFromChat}
        />
        {openFileTabs.map((tab) => (
          <FileTabContent
            key={tab.path}
            tab={tab}
            activeSession={activeSession}
            activeSessionId={activeSessionId}
            isSaving={savingFiles.has(tab.path)}
            onFileChange={handleFileChange}
            onFileSave={handleFileSave}
            onFileDelete={handleFileDelete}
          />
        ))}
      </SessionTabs>
    </SessionPanel>
  );
});

// --- Extracted sub-components ---

function ApproveButtonGroup({
  onApprove,
  onRequestChanges,
}: {
  onApprove: () => void;
  onRequestChanges: () => void;
}) {
  return (
    <div className="flex items-center gap-0.5">
      <Button
        type="button"
        size="sm"
        className="h-6 gap-1.5 px-2.5 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-medium rounded-r-none border-r border-emerald-700/30"
        onClick={onApprove}
      >
        <IconCheck className="h-3.5 w-3.5" />
        Approve
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
            Approve and continue
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={onRequestChanges}
            className="cursor-pointer text-amber-600 dark:text-amber-500"
          >
            <IconX className="h-4 w-4 mr-2" />
            Request changes
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function ChatTabContent({
  activeTaskId,
  isPassthroughMode,
  activeSessionId,
  sessionId,
  showRequestChangesTooltip,
  onDismissTooltip,
  onOpenFile,
}: {
  activeTaskId: string | null;
  isPassthroughMode: boolean;
  activeSessionId: string | null;
  sessionId: string | null | undefined;
  showRequestChangesTooltip: boolean;
  onDismissTooltip: () => void;
  onOpenFile: (filePath: string) => void;
}) {
  if (!activeTaskId) {
    return (
      <TabsContent
        value="chat"
        className="flex flex-col min-h-0 flex-1"
        style={{ minHeight: "200px" }}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          No task selected
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
          <PassthroughTerminal key={activeSessionId} sessionId={sessionId} mode="agent" />
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
        onOpenFile={onOpenFile}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onDismissTooltip}
        onOpenFileAtLine={onOpenFile}
      />
    </TabsContent>
  );
}

function FileTabContent({
  tab,
  activeSession,
  activeSessionId,
  isSaving,
  onFileChange,
  onFileSave,
  onFileDelete,
}: {
  tab: OpenFileTab;
  activeSession: { worktree_path?: string | null } | null;
  activeSessionId: string | null;
  isSaving: boolean;
  onFileChange: (path: string, content: string) => void;
  onFileSave: (path: string) => void;
  onFileDelete: (path: string) => void;
}) {
  const extCategory = getFileCategory(tab.path);
  let category: "image" | "binary" | "text" = "text";
  if (tab.isBinary) category = extCategory === "image" ? "image" : "binary";

  return (
    <TabsContent value={`file:${tab.path}`} className="flex-1 min-h-0">
      {category === "image" && (
        <FileImageViewer
          path={tab.path}
          content={tab.content}
          worktreePath={activeSession?.worktree_path ?? undefined}
        />
      )}
      {category === "binary" && (
        <FileBinaryViewer
          path={tab.path}
          worktreePath={activeSession?.worktree_path ?? undefined}
        />
      )}
      {category === "text" && (
        <FileEditorContent
          path={tab.path}
          content={tab.content}
          originalContent={tab.originalContent}
          isDirty={tab.isDirty}
          isSaving={isSaving}
          sessionId={activeSessionId || undefined}
          worktreePath={activeSession?.worktree_path ?? undefined}
          enableComments={!!activeSessionId}
          onChange={(newContent) => onFileChange(tab.path, newContent)}
          onSave={() => onFileSave(tab.path)}
          onDelete={() => onFileDelete(tab.path)}
        />
      )}
    </TabsContent>
  );
}
