'use client';

import { useCallback, useEffect, useState } from 'react';
import '@git-diff-view/react/styles/diff-view.css';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@kandev/ui/resizable';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@kandev/ui/tabs';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { Textarea } from '@kandev/ui/textarea';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { TaskChatPanel } from '@/components/task/task-chat-panel';
import { TaskTopBar } from '@/components/task/task-top-bar';
import type { Task, Comment } from '@/lib/types/http';
import { TaskFilesPanel } from '@/components/task/task-files-panel';
import { TaskChangesPanel } from '@/components/task/task-changes-panel';
import { TaskRightPanel } from '@/components/task/task-right-panel';
import { getBackendConfig } from '@/lib/config';
import { listRepositories, listRepositoryBranches } from '@/lib/http/client';
import { useRequest } from '@/lib/http/use-request';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStoreApi } from '@/components/state-provider';
import { useAppStore } from '@/components/state-provider';

const AGENTS = [
  { id: 'codex', label: 'Codex' },
  { id: 'claude', label: 'Claude Code' },
];

type TaskPageClientProps = {
  task: Task | null;
};

const DEFAULT_HORIZONTAL_LAYOUT: [number, number] = [75, 25];

export default function TaskPage({ task: initialTask }: TaskPageClientProps) {
  const store = useAppStoreApi();
  const [isMounted, setIsMounted] = useState(false);
  const [horizontalLayout, setHorizontalLayout] = useState<[number, number]>(
    getLocalStorage('task-layout-horizontal', DEFAULT_HORIZONTAL_LAYOUT)
  );
  const [leftTab, setLeftTab] = useState('chat');
  const [selectedDiffPath, setSelectedDiffPath] = useState<string | null>(null);
  const [notes, setNotes] = useState('');
  const [isLoadingComments, setIsLoadingComments] = useState(false);
  const [isAgentRunning, setIsAgentRunning] = useState(false);
  const [isAgentLoading, setIsAgentLoading] = useState(false);
  // Track worktree info separately since it's populated after agent starts
  const [worktreePath, setWorktreePath] = useState<string | null>(initialTask?.worktree_path ?? null);
  const [worktreeBranch, setWorktreeBranch] = useState<string | null>(initialTask?.worktree_branch ?? null);
  // Use task from props but allow updates
  const task = initialTask;

  // Track task state from store (kanban.tasks) to determine if agent is running
  const taskFromStore = useAppStore((state) =>
    state.kanban.tasks.find((t) => t.id === task?.id)
  );

  useEffect(() => {
    setIsMounted(true);
  }, []);

  // Fetch task execution status from orchestrator on mount
  useEffect(() => {
    if (!task?.id) return;

    const checkExecution = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      try {
        const response = await client.request<{
          has_execution: boolean;
          task_id: string;
          status?: string;
        }>('task.execution', { task_id: task.id });

        console.log('[TaskPage] Task execution check:', response);
        if (response.has_execution) {
          setIsAgentRunning(true);
        }
      } catch (err) {
        console.error('[TaskPage] Failed to check task execution:', err);
      }
    };

    checkExecution();
  }, [task?.id]);

  const fetchBranches = useCallback(async (workspaceId: string, repoPath: string) => {
    const response = await listRepositories(getBackendConfig().apiBaseUrl, workspaceId);
    const repo = response.repositories.find((item) => item.local_path === repoPath);
    if (!repo) return [];
    const branchResponse = await listRepositoryBranches(getBackendConfig().apiBaseUrl, repo.id);
    return branchResponse.branches;
  }, []);
  const branchesRequest = useRequest(fetchBranches);

  // Fetch comments on mount and when task changes
  useEffect(() => {
    if (!task?.id) return;

    // Set taskId immediately so that incoming WebSocket notifications are processed
    // before the API call completes (fixes race condition on first agent start)
    store.getState().setCommentsTaskId(task.id);

    const fetchComments = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      setIsLoadingComments(true);
      store.getState().setCommentsLoading(true);

      try {
        const response = await client.request<Comment[]>('comment.list', { task_id: task.id }, 10000);
        console.log('[API] comment.list response:', JSON.stringify(response, null, 2));
        store.getState().setComments(task.id, response);
      } catch (error) {
        console.error('Failed to fetch comments:', error);
        store.getState().setComments(task.id, []);
      } finally {
        setIsLoadingComments(false);
      }
    };

    fetchComments();
  }, [task?.id, store]);

  // Subscribe to task for real-time updates
  useEffect(() => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    // Subscribe to task updates
    client.subscribe(task.id);

    return () => {
      // Unsubscribe when leaving
      client.unsubscribe(task.id);
    };
  }, [task?.id]);

  const handleSendMessage = useCallback(async (content: string) => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    try {
      await client.request('comment.add', { task_id: task.id, content }, 10000);
    } catch (error) {
      console.error('Failed to send comment:', error);
    }
  }, [task?.id]);

  const handleSelectDiffPath = useCallback((path: string) => {
    setSelectedDiffPath(path);
    setLeftTab('changes');
  }, []);

  const handleStartAgent = useCallback(async () => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    setIsAgentLoading(true);
    try {
      // Use task's agent_type if set, otherwise default to 'augment-agent'
      const agentType = task.agent_type ?? 'augment-agent';
      interface StartResponse {
        success: boolean;
        task_id: string;
        agent_instance_id: string;
        status: string;
        worktree_path?: string;
        worktree_branch?: string;
      }
      const response = await client.request<StartResponse>('orchestrator.start', {
        task_id: task.id,
        agent_type: agentType,
      }, 15000);
      setIsAgentRunning(true);

      // Update worktree info from response
      if (response?.worktree_path) {
        setWorktreePath(response.worktree_path);
        setWorktreeBranch(response.worktree_branch ?? null);
      }
    } catch (error) {
      console.error('Failed to start agent:', error);
    } finally {
      setIsAgentLoading(false);
    }
  }, [task?.id, task?.agent_type]);

  const handleStopAgent = useCallback(async () => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    setIsAgentLoading(true);
    try {
      await client.request('orchestrator.stop', { task_id: task.id }, 15000);
      setIsAgentRunning(false);
    } catch (error) {
      console.error('Failed to stop agent:', error);
    } finally {
      setIsAgentLoading(false);
    }
  }, [task?.id]);

  const topFilesPanel = <TaskFilesPanel onSelectDiffPath={handleSelectDiffPath} />;

  useEffect(() => {
    if (!task?.workspace_id || !task.repository_url) return;
    branchesRequest.run(task.workspace_id, task.repository_url).catch(() => {});
  }, [branchesRequest.run, task?.repository_url, task?.workspace_id]);

  if (!isMounted) {
    return <div className="h-screen w-full bg-background" />;
  }

  return (
    <TooltipProvider>
      <div className="h-screen w-full flex flex-col bg-background">
      <TaskTopBar
        taskTitle={task?.title}
        baseBranch={task?.branch ?? undefined}
        branches={task?.repository_url ? branchesRequest.data ?? [] : []}
        branchesLoading={branchesRequest.isLoading}
        onStartAgent={handleStartAgent}
        onStopAgent={handleStopAgent}
        isAgentRunning={isAgentRunning}
        isAgentLoading={isAgentLoading}
        worktreePath={worktreePath}
        worktreeBranch={worktreeBranch}
      />

      <div className="flex-1 min-h-0 px-4 pb-4">
        <ResizablePanelGroup
          direction="horizontal"
          className="h-full"
          onLayout={(sizes) => {
            setHorizontalLayout(sizes as [number, number]);
            setLocalStorage('task-layout-horizontal', sizes);
          }}
        >
          <ResizablePanel defaultSize={horizontalLayout[0]} minSize={55}>
            <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-r-0 mr-[5px]">
              <Tabs
                value={leftTab}
                onValueChange={(value) => setLeftTab(value)}
                className="flex-1 min-h-0"
              >
                <TabsList>
                  <TabsTrigger value="notes" className="cursor-pointer">
                    Notes
                  </TabsTrigger>
                  <TabsTrigger value="changes" className="cursor-pointer">
                    All changes
                  </TabsTrigger>
                  <TabsTrigger value="chat" className="cursor-pointer">
                    Chat
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="notes" className="mt-3 flex-1 min-h-0">
                  <Textarea
                    value={notes}
                    onChange={(event) => setNotes(event.target.value)}
                    placeholder="Add task notes here..."
                    className="min-h-0 h-full resize-none"
                  />
                </TabsContent>

                <TabsContent value="changes" className="mt-3 flex-1 min-h-0">
                  <TaskChangesPanel
                    selectedDiffPath={selectedDiffPath}
                    onClearSelected={() => setSelectedDiffPath(null)}
                  />
                </TabsContent>

                <TabsContent value="chat" className="mt-3 flex flex-col min-h-0 flex-1">
                  {task?.id ? (
                    <TaskChatPanel
                      taskId={task.id}
                      agents={AGENTS}
                      onSend={handleSendMessage}
                      isLoading={isLoadingComments}
                      isAgentWorking={isAgentRunning}
                    />
                  ) : (
                    <div className="flex items-center justify-center h-full text-muted-foreground">
                      No task selected
                    </div>
                  )}
                </TabsContent>
              </Tabs>
            </div>
          </ResizablePanel>
          <ResizableHandle className="w-px" />
          <ResizablePanel defaultSize={horizontalLayout[1]} minSize={20}>
            <TaskRightPanel topPanel={topFilesPanel} />
          </ResizablePanel>
        </ResizablePanelGroup>
      </div>
      </div>
    </TooltipProvider>
  );
}
