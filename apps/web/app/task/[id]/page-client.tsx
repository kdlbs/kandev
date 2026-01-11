'use client';

import { useCallback, useEffect, useState } from 'react';
import '@git-diff-view/react/styles/diff-view.css';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { TooltipProvider } from '@/components/ui/tooltip';
import { Textarea } from '@/components/ui/textarea';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { TaskChatPanel } from '@/components/task/task-chat-panel';
import { TaskTopBar } from '@/components/task/task-top-bar';
import type { Task } from '@/lib/types/http';
import { TaskFilesPanel } from '@/components/task/task-files-panel';
import { TaskChangesPanel } from '@/components/task/task-changes-panel';
import { TaskRightPanel } from '@/components/task/task-right-panel';
import { getBackendConfig } from '@/lib/config';
import { listRepositories, listRepositoryBranches } from '@/lib/http/client';
import { useRequest } from '@/lib/http/use-request';

type ChatMessage = {
  id: string;
  role: 'user' | 'agent';
  content: string;
};

type ChatSession = {
  id: string;
  title: string;
  messages: ChatMessage[];
};

const AGENTS = [
  { id: 'codex', label: 'Codex' },
  { id: 'claude', label: 'Claude Code' },
];

const INITIAL_CHATS: ChatSession[] = [
  {
    id: 'chat-1',
    title: 'Build overview',
    messages: [
      {
        id: 'm1',
        role: 'agent',
        content: 'I can help with this task. What should I tackle first?',
      },
    ],
  },
];

type TaskPageClientProps = {
  task: Task | null;
};

function buildInitialChats(taskPrompt?: string): ChatSession[] {
  if (!taskPrompt) {
    return INITIAL_CHATS;
  }
  return [
    {
      ...INITIAL_CHATS[0],
      messages: [
        {
          id: 'prompt',
          role: 'user',
          content: taskPrompt,
        },
        ...INITIAL_CHATS[0].messages,
      ],
    },
  ];
}

export default function TaskPage({ task }: TaskPageClientProps) {
  const defaultHorizontalLayout: [number, number] = [75, 25];
  const [horizontalLayout, setHorizontalLayout] = useState<[number, number]>(() =>
    getLocalStorage('task-layout-horizontal', defaultHorizontalLayout)
  );
  const [chats, setChats] = useState<ChatSession[]>(() =>
    buildInitialChats(task?.description ?? '')
  );
  const [leftTab, setLeftTab] = useState('chat');
  const [selectedDiffPath, setSelectedDiffPath] = useState<string | null>(null);
  const [notes, setNotes] = useState('');
  const fetchBranches = useCallback(async (workspaceId: string, repoPath: string) => {
    const response = await listRepositories(getBackendConfig().apiBaseUrl, workspaceId);
    const repo = response.repositories.find((item) => item.local_path === repoPath);
    if (!repo) return [];
    const branchResponse = await listRepositoryBranches(getBackendConfig().apiBaseUrl, repo.id);
    return branchResponse.branches;
  }, []);
  const branchesRequest = useRequest(fetchBranches);

  const activeChat = chats[0];

  const handleSendMessage = useCallback((content: string) => {
    setChats((currentChats) => {
      const [first, ...rest] = currentChats;
      if (!first) return currentChats;
      return [
        {
          ...first,
          messages: [...first.messages, { id: crypto.randomUUID(), role: 'user', content }],
        },
        ...rest,
      ];
    });
  }, []);

  const handleSelectDiffPath = useCallback((path: string) => {
    setSelectedDiffPath(path);
    setLeftTab('changes');
  }, []);

  const topFilesPanel = <TaskFilesPanel onSelectDiffPath={handleSelectDiffPath} />;

  useEffect(() => {
    if (!task?.workspace_id || !task.repository_url) return;
    branchesRequest.run(task.workspace_id, task.repository_url).catch(() => {});
  }, [branchesRequest.run, task?.repository_url, task?.workspace_id]);

  return (
    <TooltipProvider>
      <div className="h-screen w-full flex flex-col bg-background">
      <TaskTopBar
        taskTitle={task?.title}
        baseBranch={task?.branch ?? undefined}
        branches={task?.repository_url ? branchesRequest.data ?? [] : []}
        branchesLoading={branchesRequest.isLoading}
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
                    Chat {activeChat ? `• ${activeChat.title}` : ''}
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
                  <TaskChatPanel activeChat={activeChat} agents={AGENTS} onSend={handleSendMessage} />
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
