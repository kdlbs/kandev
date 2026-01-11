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
import { TaskFilesPanel } from '@/components/task/task-files-panel';
import { TaskChangesPanel } from '@/components/task/task-changes-panel';
import { TaskRightPanel } from '@/components/task/task-right-panel';
import { useAppStoreApi } from '@/components/state-provider';
import { useWebSocket } from '@/lib/ws/use-websocket';

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
      {
        id: 'm2',
        role: 'user',
        content: 'Please review the changes and summarize the key diffs.',
      },
    ],
  },
];


export default function TaskPage() {
  const store = useAppStoreApi();
  useWebSocket(store, 'ws://localhost:8080/ws');
  const defaultHorizontalLayout: [number, number] = [75, 25];
  const [horizontalLayout, setHorizontalLayout] = useState(defaultHorizontalLayout);
  const [horizontalSeed, setHorizontalSeed] = useState(0);
  const [chats, setChats] = useState<ChatSession[]>(INITIAL_CHATS);
  const [leftTab, setLeftTab] = useState('chat');
  const [selectedDiffPath, setSelectedDiffPath] = useState<string | null>(null);
  const [notes, setNotes] = useState('');

  const activeChat = chats[0];

  useEffect(() => {
    const storedHorizontal = getLocalStorage('task-layout-horizontal', defaultHorizontalLayout);

    if (
      storedHorizontal[0] !== defaultHorizontalLayout[0] ||
      storedHorizontal[1] !== defaultHorizontalLayout[1]
    ) {
      setHorizontalLayout(storedHorizontal);
      setHorizontalSeed((value) => value + 1);
    }
  }, []);

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

  return (
    <TooltipProvider>
      <div className="h-screen w-full flex flex-col bg-background">
      <TaskTopBar />

      <div className="flex-1 min-h-0 px-4 pb-4">
        <ResizablePanelGroup
          key={horizontalSeed}
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
