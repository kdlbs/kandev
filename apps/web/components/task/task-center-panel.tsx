'use client';

import { memo, useCallback, useState, useEffect, useMemo, useRef } from 'react';
import { IconCheck, IconChevronDown, IconX } from '@tabler/icons-react';
import { TabsContent } from '@kandev/ui/tabs';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from '@kandev/ui/dropdown-menu';
import { SessionPanel } from '@kandev/ui/pannel-session';
import { TaskChatPanel } from './task-chat-panel';
import { TaskChangesPanel } from './task-changes-panel';
import { FileEditorContent } from './file-editor-content';
import { FileImageViewer } from './file-image-viewer';
import { FileBinaryViewer } from './file-binary-viewer';
import { PassthroughTerminal } from './passthrough-terminal';
import type { OpenFileTab, FileContentResponse } from '@/lib/types/backend';
import { useAppStore } from '@/components/state-provider';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';
import { approveSessionAction } from '@/app/actions/workspaces';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileContent, updateFileContent, deleteFile } from '@/lib/ws/workspace-files';
import { getOpenFileTabs, setOpenFileTabs as saveOpenFileTabs, getActiveTabForSession, setActiveTabForSession } from '@/lib/local-storage';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';
import { generateUnifiedDiff, calculateHash } from '@/lib/utils/file-diff';
import { getFileCategory } from '@/lib/utils/file-types';
import { useToast } from '@/components/toast-provider';

import type { SelectedDiff } from './task-layout';

type TaskCenterPanelProps = {
  selectedDiff: SelectedDiff | null;
  openFileRequest: OpenFileTab | null;
  onDiffHandled: () => void;
  onFileOpenHandled: () => void;
  onActiveFileChange?: (filePath: string | null) => void;
  sessionId?: string | null;
};

export const TaskCenterPanel = memo(function TaskCenterPanel({
  selectedDiff: externalSelectedDiff,
  openFileRequest,
  onDiffHandled,
  onFileOpenHandled,
  onActiveFileChange,
  sessionId = null,
}: TaskCenterPanelProps) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const setTaskSession = useAppStore((state) => state.setTaskSession);

  // Check if agent is currently working
  const isAgentWorking = activeSession?.state === 'STARTING' || activeSession?.state === 'RUNNING';

  // Check if session is in passthrough mode by looking at the profile snapshot
  const isPassthroughMode = useMemo(() => {
    if (!activeSession?.agent_profile_snapshot) return false;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const snapshot = activeSession.agent_profile_snapshot as any;
    return snapshot?.cli_passthrough === true;
  }, [activeSession?.agent_profile_snapshot]);

  // Check if we should show the approve button
  // Show when session has a review_status that is not approved (meaning it's in a review step)
  // Also hide while agent is working to prevent premature approval
  // Use truthy check to handle null, undefined, and empty string (backend may send "" when clearing)
  const showApproveButton =
    !!activeSession?.review_status &&
    activeSession.review_status !== 'approved' &&
    !isAgentWorking;

  // Approve handler - moves session to next workflow step
  const handleApprove = useCallback(async () => {
    if (!activeSessionId || !activeTaskId) return;
    try {
      const response = await approveSessionAction(activeSessionId);

      // Update the session in the store so the review panel closes
      if (response?.session) {
        setTaskSession(response.session);
      }

      // Check if the new step has auto_start_agent enabled
      if (response?.workflow_step?.events?.on_enter?.some((a: { type: string }) => a.type === 'auto_start_agent')) {
        const client = getWebSocketClient();
        if (client) {
          client.send({
            type: 'request',
            action: 'orchestrator.start',
            payload: {
              task_id: activeTaskId,
              session_id: activeSessionId,
              workflow_step_id: response.workflow_step.id,
            },
          });
        }
      }
    } catch (error) {
      console.error('Failed to approve session:', error);
    }
  }, [activeSessionId, activeTaskId, setTaskSession]);

  // Get git status and commits to determine if there are changes
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId);
  const hasChanges = useMemo(() => {
    const hasUncommittedChanges = gitStatus?.files && Object.keys(gitStatus.files).length > 0;
    const hasCommits = commits && commits.length > 0;
    return hasUncommittedChanges || hasCommits;
  }, [gitStatus, commits]);

  // Initialize tab - restored from sessionStorage or default to 'chat'
  const [leftTab, setLeftTab] = useState(() => {
    // Try to restore from session-specific storage on initial render
    if (typeof window !== 'undefined' && activeSessionId) {
      const savedTab = getActiveTabForSession(activeSessionId, 'chat');
      // Only return non-file tabs synchronously, file tabs need async content loading
      if (savedTab === 'chat' || savedTab === 'changes') {
        return savedTab;
      }
    }
    return 'chat';
  });

  // If current tab is 'changes' but there are no changes, switch to 'chat'
  useEffect(() => {
    if (leftTab === 'changes' && !hasChanges) {
      setLeftTab('chat');
    }
  }, [leftTab, hasChanges]);

  // Listen for external requests to switch to 'changes' tab
  useEffect(() => {
    const handler = () => {
      if (hasChanges) {
        setLeftTab('changes');
      }
    };
    window.addEventListener('switch-to-changes-tab', handler);
    return () => window.removeEventListener('switch-to-changes-tab', handler);
  }, [hasChanges]);

  // Request changes state - triggers focus and tooltip on chat input
  const [showRequestChangesTooltip, setShowRequestChangesTooltip] = useState(false);

  // Request changes handler - switches to chat and focuses input
  const handleRequestChanges = useCallback(() => {
    // Switch to chat tab
    setLeftTab('chat');
    // Show tooltip on input
    setShowRequestChangesTooltip(true);
    // Auto-hide tooltip after 5 seconds
    setTimeout(() => setShowRequestChangesTooltip(false), 5000);
  }, []);
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);
  const [openFileTabs, setOpenFileTabs] = useState<OpenFileTab[]>([]);
  const [savingFiles, setSavingFiles] = useState<Set<string>>(new Set());
  const restoredTabsRef = useRef<string | null>(null);
  const restorationInProgressRef = useRef<boolean>(false);
  const { toast } = useToast();

  // Save current active tab for the current session before switching
  const prevSessionRef = useRef<string | null>(null);
  useEffect(() => {
    // On unmount or before session change, save the current tab for the previous session
    return () => {
      if (prevSessionRef.current && leftTab) {
        setActiveTabForSession(prevSessionRef.current, leftTab);
      }
    };
  }, [leftTab]);

  // Restore file tabs and active tab from sessionStorage on mount/session change
  useEffect(() => {
    if (!activeSessionId) return;

    // When session changes, always clear existing tabs first and reset state
    if (restoredTabsRef.current !== activeSessionId) {
      // Save the active tab for the OLD session before switching
      if (prevSessionRef.current && prevSessionRef.current !== activeSessionId) {
        setActiveTabForSession(prevSessionRef.current, leftTab);
      }

      restoredTabsRef.current = activeSessionId;
      prevSessionRef.current = activeSessionId;
      restorationInProgressRef.current = false;

      // Clear existing file tabs immediately when session changes
      setOpenFileTabs([]);
    } else if (restorationInProgressRef.current) {
      // Already restoring for this session
      return;
    } else {
      // Already restored for this session
      return;
    }

    const savedTabs = getOpenFileTabs(activeSessionId);
    const savedActiveTab = getActiveTabForSession(activeSessionId, 'chat');

    // If no file tabs to load, just restore the active tab
    if (savedTabs.length === 0) {
      // Only set if it's a valid non-file tab
      if (savedActiveTab === 'chat' || savedActiveTab === 'changes') {
        setLeftTab(savedActiveTab);
      } else {
        setLeftTab('chat');
      }
      return;
    }

    // Mark restoration as in progress
    restorationInProgressRef.current = true;

    // Load content for each saved tab with retry for WebSocket client
    const loadTabs = async (retryCount = 0): Promise<void> => {
      const maxRetries = 5;
      const retryDelay = 200;

      const client = getWebSocketClient();
      if (!client) {
        if (retryCount < maxRetries) {
          setTimeout(() => loadTabs(retryCount + 1), retryDelay);
          return;
        }
        restorationInProgressRef.current = false;
        return;
      }

      // Verify we're still restoring for the same session
      if (restoredTabsRef.current !== activeSessionId) {
        restorationInProgressRef.current = false;
        return;
      }

      const loadedTabs: OpenFileTab[] = [];
      for (const savedTab of savedTabs) {
        try {
          const response = await requestFileContent(client, activeSessionId, savedTab.path);
          const hash = await calculateHash(response.content);
          loadedTabs.push({
            path: savedTab.path,
            name: savedTab.name,
            content: response.content,
            originalContent: response.content,
            originalHash: hash,
            isDirty: false,
            isBinary: response.is_binary,
          });
        } catch {
          // Failed to restore tab, skip it
        }
      }

      // Verify session hasn't changed
      if (restoredTabsRef.current !== activeSessionId) {
        restorationInProgressRef.current = false;
        return;
      }

      if (loadedTabs.length > 0) {
        setOpenFileTabs(loadedTabs);

        // Restore active tab - check if it's a file tab that was loaded
        if (savedActiveTab.startsWith('file:')) {
          const filePath = savedActiveTab.replace('file:', '');
          const tabExists = loadedTabs.some(t => t.path === filePath);
          if (tabExists) {
            // Use setTimeout to ensure React has processed setOpenFileTabs
            setTimeout(() => {
              setLeftTab(savedActiveTab);
              restorationInProgressRef.current = false;
            }, 0);
          } else {
            setLeftTab('chat');
            restorationInProgressRef.current = false;
          }
        } else {
          setLeftTab(savedActiveTab);
          restorationInProgressRef.current = false;
        }
      } else {
        // No tabs could be loaded, fall back to non-file tab
        if (savedActiveTab === 'chat' || savedActiveTab === 'changes') {
          setLeftTab(savedActiveTab);
        } else {
          setLeftTab('chat');
        }
        restorationInProgressRef.current = false;
      }
    };

    void loadTabs();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- leftTab is intentionally excluded to prevent re-running on tab changes
  }, [activeSessionId]);

  // Persist open file tabs to sessionStorage when they change
  useEffect(() => {
    if (!activeSessionId) return;
    const tabsToSave = openFileTabs.map(({ path, name }) => ({ path, name }));
    saveOpenFileTabs(activeSessionId, tabsToSave);
  }, [activeSessionId, openFileTabs]);

  // Persist active tab whenever it changes (not just via handleTabChange)
  useEffect(() => {
    if (!activeSessionId) return;
    // Skip during restoration to avoid overwriting with intermediate values
    if (restorationInProgressRef.current) {
      return;
    }
    setActiveTabForSession(activeSessionId, leftTab);
  }, [activeSessionId, leftTab]);

  // Notify parent when the active file changes
  useEffect(() => {
    if (leftTab.startsWith('file:')) {
      const filePath = leftTab.replace('file:', '');
      onActiveFileChange?.(filePath);
    } else {
      onActiveFileChange?.(null);
    }
  }, [leftTab, onActiveFileChange]);

  // Handle tab change
  const handleTabChange = useCallback((tab: string) => {
    setLeftTab(tab);
    // Persist tab selection per-session (for all tabs including file tabs)
    if (activeSessionId) {
      setActiveTabForSession(activeSessionId, tab);
    }
  }, [activeSessionId]);

  // Handle external diff selection
  useEffect(() => {
    if (externalSelectedDiff) {
      queueMicrotask(() => {
        setSelectedDiff(externalSelectedDiff);
        setLeftTab('changes');
        onDiffHandled();
      });
    }
  }, [externalSelectedDiff, onDiffHandled]);

  // Handle external file open request
  useEffect(() => {
    if (openFileRequest) {
      queueMicrotask(async () => {
        // Calculate hash for the file if not already present
        const hash = openFileRequest.originalHash || await calculateHash(openFileRequest.content);
        const fileWithHash: OpenFileTab = {
          ...openFileRequest,
          originalContent: openFileRequest.originalContent || openFileRequest.content,
          originalHash: hash,
          isDirty: openFileRequest.isDirty ?? false,
        };

        setOpenFileTabs((prev) => {
          // If file is already open, just switch to it
          if (prev.some((t) => t.path === fileWithHash.path)) {
            return prev;
          }
          // Add new tab (LRU eviction at max 4)
          const maxTabs = 4;
          const newTabs = prev.length >= maxTabs ? [...prev.slice(1), fileWithHash] : [...prev, fileWithHash];
          return newTabs;
        });
        setLeftTab(`file:${fileWithHash.path}`);
        onFileOpenHandled();
      });
    }
  }, [openFileRequest, onFileOpenHandled]);

  const handleCloseFileTab = useCallback((path: string) => {
    setOpenFileTabs((prev) => prev.filter((t) => t.path !== path));
    // If closing the active tab, switch to chat
    if (leftTab === `file:${path}`) {
      handleTabChange('chat');
    }
  }, [leftTab, handleTabChange]);

  // Handler for opening files from chat (tool read messages)
  const handleOpenFileFromChat = useCallback(async (filePath: string) => {
    const client = getWebSocketClient();
    const currentSessionId = activeSessionId;
    if (!client || !currentSessionId) return;

    try {
      const response: FileContentResponse = await requestFileContent(client, currentSessionId, filePath);
      const fileName = filePath.split('/').pop() || filePath;
      const hash = await calculateHash(response.content);

      setOpenFileTabs((prev) => {
        // If file is already open, just switch to it
        if (prev.some((t) => t.path === filePath)) {
          return prev;
        }
        // Add new tab (LRU eviction at max 4)
        const maxTabs = 4;
        const newTab: OpenFileTab = {
          path: filePath,
          name: fileName,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
        };
        const newTabs = prev.length >= maxTabs ? [...prev.slice(1), newTab] : [...prev, newTab];
        return newTabs;
      });
      setLeftTab(`file:${filePath}`);
    } catch (error) {
      const reason = error instanceof Error ? error.message : 'Unknown error';
      toast({
        title: 'Failed to open file',
        description: reason,
        variant: 'error',
      });
    }
  }, [activeSessionId, toast]);

  // Handler for file content changes
  const handleFileChange = useCallback((path: string, newContent: string) => {
    setOpenFileTabs((prev) =>
      prev.map((tab) =>
        tab.path === path
          ? { ...tab, content: newContent, isDirty: newContent !== tab.originalContent }
          : tab
      )
    );
  }, []);

  // Handler for saving file
  const handleFileSave = useCallback(async (path: string) => {
    const tab = openFileTabs.find((t) => t.path === path);
    if (!tab || !tab.isDirty) return;

    const client = getWebSocketClient();
    if (!client || !activeSessionId) return;

    setSavingFiles((prev) => new Set(prev).add(path));

    try {
      const diff = generateUnifiedDiff(tab.originalContent, tab.content, tab.path);
      const response = await updateFileContent(client, activeSessionId, path, diff, tab.originalHash);

      if (response.success && response.new_hash) {
        setOpenFileTabs((prev) =>
          prev.map((t) =>
            t.path === path
              ? { ...t, originalContent: t.content, originalHash: response.new_hash!, isDirty: false }
              : t
          )
        );
      } else {
        toast({
          title: 'Save failed',
          description: response.error || 'Failed to save file',
          variant: 'error',
        });
      }
    } catch (error) {
      console.error('Failed to save file:', error);
      toast({
        title: 'Save failed',
        description: error instanceof Error ? error.message : 'An error occurred while saving the file',
        variant: 'error',
      });
    } finally {
      setSavingFiles((prev) => {
        const next = new Set(prev);
        next.delete(path);
        return next;
      });
    }
  }, [openFileTabs, activeSessionId, toast]);

  // Handler for deleting file from editor
  const handleFileDelete = useCallback(async (path: string) => {
    const client = getWebSocketClient();
    if (!client || !activeSessionId) return;

    try {
      const response = await deleteFile(client, activeSessionId, path);
      if (response.success) {
        // Close the tab
        handleCloseFileTab(path);
      } else {
        toast({
          title: 'Delete failed',
          description: response.error || 'Failed to delete file',
          variant: 'error',
        });
      }
    } catch (error) {
      console.error('Failed to delete file:', error);
      toast({
        title: 'Delete failed',
        description: error instanceof Error ? error.message : 'An error occurred while deleting the file',
        variant: 'error',
      });
    }
  }, [activeSessionId, handleCloseFileTab, toast]);

  const tabs: SessionTab[] = useMemo(() => {
    const staticTabs: SessionTab[] = [
      // Only show "All changes" tab if there are changes
      ...(hasChanges ? [{ id: 'changes', label: 'All changes' }] : []),
      { id: 'chat', label: 'Chat' },
    ];

    const fileTabs: SessionTab[] = openFileTabs.map((tab) => {
      return {
        id: `file:${tab.path}`,
        label: tab.isDirty ? `${tab.name} *` : tab.name,
        icon: tab.isDirty ? <span className="h-2 w-2 rounded-full bg-yellow-500" /> : undefined,
        closable: true,
        onClose: (e) => {
          e.stopPropagation();
          handleCloseFileTab(tab.path);
        },
        className: 'cursor-pointer group gap-1.5 data-[state=active]:bg-muted',
      };
    });

    return [...staticTabs, ...fileTabs];
  }, [openFileTabs, handleCloseFileTab, hasChanges]);

  return (
    <SessionPanel borderSide="right" margin="right">
      <SessionTabs
        tabs={tabs}
        activeTab={leftTab}
        onTabChange={handleTabChange}
        separatorAfterIndex={openFileTabs.length > 0 ? (hasChanges ? 1 : 0) : undefined}
        className="flex-1 min-h-0 flex flex-col gap-2"
        rightContent={
          showApproveButton ? (
            <div className="flex items-center gap-0.5">
              <Button
                type="button"
                size="sm"
                className="h-6 gap-1.5 px-2.5 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-medium rounded-r-none border-r border-emerald-700/30"
                onClick={handleApprove}
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
                  <DropdownMenuItem onClick={handleApprove} className="cursor-pointer">
                    <IconCheck className="h-4 w-4 mr-2" />
                    Approve and continue
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={handleRequestChanges}
                    className="cursor-pointer text-amber-600 dark:text-amber-500"
                  >
                    <IconX className="h-4 w-4 mr-2" />
                    Request changes
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ) : undefined
        }
      >

        <TabsContent value="changes" className="flex-1 min-h-0">
          <TaskChangesPanel
            selectedDiff={selectedDiff}
            onClearSelected={() => setSelectedDiff(null)}
            onOpenFile={handleOpenFileFromChat}
          />
        </TabsContent>

        <TabsContent value="chat" className="flex flex-col min-h-0 flex-1" style={{ minHeight: '200px' }}>
          {activeTaskId ? (
            isPassthroughMode ? (
              <div className="flex-1 min-h-0 h-full" style={{ minHeight: '150px' }}>
                <PassthroughTerminal key={activeSessionId} sessionId={sessionId} terminalId="default" />
              </div>
            ) : (
              <TaskChatPanel
                sessionId={sessionId}
                onOpenFile={handleOpenFileFromChat}
                showRequestChangesTooltip={showRequestChangesTooltip}
                onRequestChangesTooltipDismiss={() => setShowRequestChangesTooltip(false)}
                onOpenFileAtLine={(filePath) => {
                  // Open the file in editor tab (or switch to it)
                  handleOpenFileFromChat(filePath);
                }}
              />
            )
          ) : (
            <div className="flex items-center justify-center h-full text-muted-foreground">
              No task selected
            </div>
          )}
        </TabsContent>

        {openFileTabs.map((tab) => {
          const extCategory = getFileCategory(tab.path);
          const category = tab.isBinary
            ? (extCategory === 'image' ? 'image' : 'binary')
            : 'text';

          return (
            <TabsContent key={tab.path} value={`file:${tab.path}`} className="flex-1 min-h-0">
              {category === 'image' ? (
                <FileImageViewer
                  path={tab.path}
                  content={tab.content}
                  worktreePath={activeSession?.worktree_path ?? undefined}
                />
              ) : category === 'binary' ? (
                <FileBinaryViewer
                  path={tab.path}
                  worktreePath={activeSession?.worktree_path ?? undefined}
                />
              ) : (
                <FileEditorContent
                  path={tab.path}
                  originalContent={tab.originalContent}
                  isDirty={tab.isDirty}
                  isSaving={savingFiles.has(tab.path)}
                  sessionId={activeSessionId || undefined}
                  worktreePath={activeSession?.worktree_path ?? undefined}
                  enableComments={!!activeSessionId}
                  onChange={(newContent) => handleFileChange(tab.path, newContent)}
                  onSave={() => handleFileSave(tab.path)}
                  onDelete={() => handleFileDelete(tab.path)}
                />
              )}
            </TabsContent>
          );
        })}
      </SessionTabs>
    </SessionPanel>
  );
});
