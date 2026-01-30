'use client';

import { memo, useCallback, useState, useEffect, useMemo } from 'react';
import { TabsContent } from '@kandev/ui/tabs';
import { Textarea } from '@kandev/ui/textarea';
import { SessionPanel } from '@kandev/ui/pannel-session';
import { TaskChatPanel } from './task-chat-panel';
import { TaskChangesPanel } from './task-changes-panel';
import { TaskPlanPanel } from './task-plan-panel';
import { FileViewerContent } from './file-viewer-content';
import { PassthroughTerminal } from './passthrough-terminal';
import type { OpenFileTab, FileContentResponse } from '@/lib/types/backend';
import { FILE_EXTENSION_COLORS } from '@/lib/types/backend';
import { useAppStore } from '@/components/state-provider';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';
import { approveSessionAction } from '@/app/actions/workspaces';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileContent } from '@/lib/ws/workspace-files';

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
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);
  const [notes, setNotes] = useState('');
  const [openFileTabs, setOpenFileTabs] = useState<OpenFileTab[]>([]);

  // Track plan updates for notification dot
  const plan = useAppStore((state) =>
    activeTaskId ? state.taskPlans.byTaskId[activeTaskId] : null
  );
  // Track the last plan update timestamp the user has seen (when they viewed the Plan tab)
  // Key is taskId to handle task switching
  const [lastSeenPlanUpdateByTask, setLastSeenPlanUpdateByTask] = useState<Record<string, string | null>>({});

  // Derive notification state: show dot if plan was updated by agent and we haven't seen it
  const hasUnseenPlanUpdate = useMemo(() => {
    if (!activeTaskId || !plan || leftTab === 'plan') return false;
    if (plan.created_by !== 'agent') return false;
    const lastSeen = lastSeenPlanUpdateByTask[activeTaskId];
    return plan.updated_at !== lastSeen;
  }, [activeTaskId, plan, leftTab, lastSeenPlanUpdateByTask]);

  // Handle tab change - mark plan as seen when switching to plan tab
  const handleTabChange = useCallback((tab: string) => {
    // If switching to plan tab, mark current plan as seen
    if (tab === 'plan' && activeTaskId && plan?.updated_at) {
      setLastSeenPlanUpdateByTask(prev => ({
        ...prev,
        [activeTaskId]: plan.updated_at
      }));
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
      queueMicrotask(() => {
        setOpenFileTabs((prev) => {
          // If file is already open, just switch to it
          if (prev.some((t) => t.path === openFileRequest.path)) {
            return prev;
          }
          // Add new tab (LRU eviction at max 4)
          const maxTabs = 4;
          const newTabs = prev.length >= maxTabs ? [...prev.slice(1), openFileRequest] : [...prev, openFileRequest];
          return newTabs;
        });
        setLeftTab(`file:${openFileRequest.path}`);
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
        };
        const newTabs = prev.length >= maxTabs ? [...prev.slice(1), newTab] : [...prev, newTab];
        return newTabs;
      });
      setLeftTab(`file:${filePath}`);
    } catch (error) {
      console.error('Failed to open file from chat:', error);
    }
  }, [activeSessionId]);

  const tabs: SessionTab[] = useMemo(() => {
    const staticTabs: SessionTab[] = [
      {
        id: 'plan',
        label: 'Plan',
        icon: hasUnseenPlanUpdate ? (
          <span className="h-2 w-2 rounded-full bg-primary animate-pulse" />
        ) : undefined,
      },
      { id: 'notes', label: 'Notes' },
      { id: 'changes', label: 'All changes' },
      { id: 'chat', label: 'Chat' },
    ];

    const fileTabs: SessionTab[] = openFileTabs.map((tab) => {
      const ext = tab.name.split('.').pop()?.toLowerCase() || '';
      const dotColor = FILE_EXTENSION_COLORS[ext] || 'bg-muted-foreground';
      return {
        id: `file:${tab.path}`,
        label: tab.name,
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
        separatorAfterIndex={openFileTabs.length > 0 ? 3 : undefined}
        className="flex-1 min-h-0 flex flex-col gap-2"
      >

        <TabsContent value="plan" className="flex-1 min-h-0" forceMount style={{ display: leftTab === 'plan' ? undefined : 'none' }}>
          <TaskPlanPanel
            taskId={activeTaskId}
            showApproveButton={showApproveButton}
            onApprove={handleApprove}
            visible={leftTab === 'plan'}
          />
        </TabsContent>

        <TabsContent value="notes" className="flex-1 min-h-0">
          <Textarea
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
            placeholder="Add task notes here..."
            className="min-h-0 h-full resize-none"
          />
        </TabsContent>

        <TabsContent value="changes" className="flex-1 min-h-0">
          <TaskChangesPanel
            selectedDiff={selectedDiff}
            onClearSelected={() => setSelectedDiff(null)}
          />
        </TabsContent>

        <TabsContent value="chat" className="flex flex-col min-h-0 flex-1" style={{ minHeight: '200px' }}>
          {activeTaskId ? (
            isPassthroughMode ? (
              <div className="flex-1 min-h-0 h-full" style={{ minHeight: '150px' }}>
                <PassthroughTerminal key={activeSessionId} sessionId={sessionId} />
              </div>
            ) : (
              <TaskChatPanel sessionId={sessionId} onOpenFile={handleOpenFileFromChat} />
            )
          ) : (
            <div className="flex items-center justify-center h-full text-muted-foreground">
              No task selected
            </div>
          )}
        </TabsContent>

        {openFileTabs.map((tab) => (
          <TabsContent key={tab.path} value={`file:${tab.path}`} className="mt-3 flex-1 min-h-0">
            <FileViewerContent path={tab.path} content={tab.content} />
          </TabsContent>
        ))}
      </SessionTabs>
    </SessionPanel>
  );
});
