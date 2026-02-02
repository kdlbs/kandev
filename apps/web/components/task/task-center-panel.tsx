'use client';

import { memo, useCallback, useState, useEffect, useMemo } from 'react';
import { IconSparkles, IconCheck, IconChevronDown, IconX } from '@tabler/icons-react';
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
import { TaskPlanPanel } from './task-plan-panel';
import { FileViewerContent } from './file-viewer-content';
import { FileEditorContent } from './file-editor-content';
import { PassthroughTerminal } from './passthrough-terminal';
import type { OpenFileTab, FileContentResponse } from '@/lib/types/backend';
import { FILE_EXTENSION_COLORS } from '@/lib/types/backend';
import { useAppStore } from '@/components/state-provider';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';
import { approveSessionAction } from '@/app/actions/workspaces';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileContent, updateFileContent } from '@/lib/ws/workspace-files';
import { getPlanLastSeen, setPlanLastSeen } from '@/lib/local-storage';
import { generateUnifiedDiff, calculateHash } from '@/lib/utils/file-diff';
import { useToast } from '@/components/toast-provider';

import type { SelectedDiff } from './task-layout';

type TaskCenterPanelProps = {
  selectedDiff: SelectedDiff | null;
  openFileRequest: OpenFileTab | null;
  onDiffHandled: () => void;
  onFileOpenHandled: () => void;
  sessionId?: string | null;
};

export const TaskCenterPanel = memo(function TaskCenterPanel({
  selectedDiff: externalSelectedDiff,
  openFileRequest,
  onDiffHandled,
  onFileOpenHandled,
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
  const showApproveButton =
    activeSession?.review_status != null && activeSession.review_status !== 'approved' && !isAgentWorking;

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
      if (response?.workflow_step?.auto_start_agent) {
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

  const [leftTab, setLeftTab] = useState('chat');

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
  const { toast } = useToast();

  // Track plan updates for notification dot
  const plan = useAppStore((state) =>
    activeTaskId ? state.taskPlans.byTaskId[activeTaskId] : null
  );

  // Derive notification state: show dot if plan was updated by agent and we haven't seen it
  const hasUnseenPlanUpdate = useMemo(() => {
    if (!activeTaskId || !plan || leftTab === 'plan') return false;
    if (plan.created_by !== 'agent') return false;
    const lastSeen = getPlanLastSeen(activeTaskId);
    return plan.updated_at !== lastSeen;
  }, [activeTaskId, plan, leftTab]);

  // Handle tab change - mark plan as seen when switching to plan tab
  const handleTabChange = useCallback((tab: string) => {
    // If switching to plan tab, mark current plan as seen in localStorage
    if (tab === 'plan' && activeTaskId && plan?.updated_at) {
      setPlanLastSeen(activeTaskId, plan.updated_at);
    }
    setLeftTab(tab);
  }, [activeTaskId, plan]);

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
        };
        const newTabs = prev.length >= maxTabs ? [...prev.slice(1), newTab] : [...prev, newTab];
        return newTabs;
      });
      setLeftTab(`file:${filePath}`);
    } catch (error) {
      console.error('Failed to open file from chat:', error);
    }
  }, [activeSessionId]);

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
      const diff = generateUnifiedDiff(tab.originalContent, tab.content, tab.name);
      const response = await updateFileContent(client, activeSessionId, path, diff, tab.originalHash);

      if (response.success && response.new_hash) {
        setOpenFileTabs((prev) =>
          prev.map((t) =>
            t.path === path
              ? { ...t, originalContent: t.content, originalHash: response.new_hash!, isDirty: false }
              : t
          )
        );
        toast({
          title: 'File saved',
          description: `${tab.name} has been saved successfully.`,
        });
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

  const tabs: SessionTab[] = useMemo(() => {
    const staticTabs: SessionTab[] = [
      { id: 'changes', label: 'All changes' },
      { id: 'chat', label: 'Chat' },
      {
        id: 'plan',
        label: 'Plan',
        icon: hasUnseenPlanUpdate ? (
          <div className="relative">
            <IconSparkles className="h-3.5 w-3.5 text-amber-500 dark:text-amber-400 animate-pulse" />
            <span className="absolute -top-0.5 -right-0.5 h-1.5 w-1.5 rounded-full bg-amber-500 dark:bg-amber-400 animate-ping" />
          </div>
        ) : undefined,
      },
    ];

    const fileTabs: SessionTab[] = openFileTabs.map((tab) => {
      const ext = tab.name.split('.').pop()?.toLowerCase() || '';
      const dotColor = tab.isDirty ? 'bg-yellow-500' : (FILE_EXTENSION_COLORS[ext] || 'bg-muted-foreground');
      return {
        id: `file:${tab.path}`,
        label: tab.isDirty ? `${tab.name} *` : tab.name,
        icon: <span className={`h-2 w-2 rounded-full ${dotColor}`} />,
        closable: true,
        onClose: (e) => {
          e.stopPropagation();
          handleCloseFileTab(tab.path);
        },
        className: 'cursor-pointer group gap-1.5 data-[state=active]:bg-muted',
      };
    });

    return [...staticTabs, ...fileTabs];
  }, [openFileTabs, handleCloseFileTab, hasUnseenPlanUpdate]);

  return (
    <SessionPanel borderSide="right" margin="right">
      <SessionTabs
        tabs={tabs}
        activeTab={leftTab}
        onTabChange={handleTabChange}
        separatorAfterIndex={openFileTabs.length > 0 ? 2 : undefined}
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

        <TabsContent value="plan" className="flex-1 min-h-0" forceMount style={{ display: leftTab === 'plan' ? undefined : 'none' }}>
          <TaskPlanPanel
            taskId={activeTaskId}
            visible={leftTab === 'plan'}
          />
        </TabsContent>

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
                <PassthroughTerminal key={activeSessionId} sessionId={sessionId} />
              </div>
            ) : (
              <TaskChatPanel
                sessionId={sessionId}
                onOpenFile={handleOpenFileFromChat}
                showRequestChangesTooltip={showRequestChangesTooltip}
                onRequestChangesTooltipDismiss={() => setShowRequestChangesTooltip(false)}
                onSelectDiff={(path) => {
                  setSelectedDiff({ path });
                  setLeftTab('changes');
                }}
              />
            )
          ) : (
            <div className="flex items-center justify-center h-full text-muted-foreground">
              No task selected
            </div>
          )}
        </TabsContent>

        {openFileTabs.map((tab) => (
          <TabsContent key={tab.path} value={`file:${tab.path}`} className="mt-3 flex-1 min-h-0">
            <FileEditorContent
              path={tab.path}
              content={tab.content}
              originalContent={tab.originalContent}
              isDirty={tab.isDirty}
              isSaving={savingFiles.has(tab.path)}
              onChange={(newContent) => handleFileChange(tab.path, newContent)}
              onSave={() => handleFileSave(tab.path)}
            />
          </TabsContent>
        ))}
      </SessionTabs>
    </SessionPanel>
  );
});
