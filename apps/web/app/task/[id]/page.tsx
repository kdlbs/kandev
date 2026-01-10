'use client';

import { FormEvent, useState } from 'react';
import Link from 'next/link';
import {
  IconArrowDown,
  IconArrowLeft,
  IconBrandVscode,
  IconChevronDown,
  IconChevronRight,
  IconChevronUp,
  IconCopy,
  IconEye,
  IconGitBranch,
  IconGitFork,
  IconGitMerge,
  IconGitPullRequest,
  IconPencil,
  IconX,
} from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { cn } from '@/lib/utils';
import { CommitStatBadge, LineStat } from '@/components/diff-stat';

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

const CHANGED_FILES = [
  { path: 'apps/web/components/kanban-board.tsx', status: 'M', plus: 12, minus: 4 },
  { path: 'apps/web/components/kanban-card.tsx', status: 'M', plus: 3, minus: 2 },
  { path: 'apps/web/components/task-create-dialog.tsx', status: 'A', plus: 55, minus: 0 },
  { path: 'apps/web/components/kanban-column.tsx', status: 'D', plus: 0, minus: 18 },
];

const ALL_FILES = [
  'apps/web/app/page.tsx',
  'apps/web/app/task/[id]/page.tsx',
  'apps/web/components/kanban-board.tsx',
  'apps/web/components/kanban-card.tsx',
  'apps/web/components/kanban-column.tsx',
  'apps/web/components/task-create-dialog.tsx',
];

const COMMANDS = [
  { id: 'dev', label: 'npm run dev' },
  { id: 'lint', label: 'npm run lint' },
  { id: 'test', label: 'npm run test' },
];

const badgeClass = (status: string) =>
  cn(
    'text-[10px] font-semibold',
    status === 'M' && 'bg-yellow-500/15 text-yellow-700',
    status === 'A' && 'bg-emerald-500/15 text-emerald-700',
    status === 'D' && 'bg-rose-500/15 text-rose-700'
  );

const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf('/');
  if (lastSlash === -1) return { folder: '', file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

export default function TaskPage() {
  const [selectedAgent, setSelectedAgent] = useState(AGENTS[0].id);
  const [chats, setChats] = useState<ChatSession[]>(INITIAL_CHATS);
  const [leftTab, setLeftTab] = useState<'notes' | 'changes' | 'chat'>('chat');
  const [messageInput, setMessageInput] = useState('');
  const [topTab, setTopTab] = useState<'diff' | 'files'>('diff');
  const [activeTerminalId, setActiveTerminalId] = useState(1);
  const [terminalIds, setTerminalIds] = useState([1]);
  const [notes, setNotes] = useState('');
  const [isBottomCollapsed, setIsBottomCollapsed] = useState(false);
  const [branchName, setBranchName] = useState('feature/agent-ui');
  const [isEditingBranch, setIsEditingBranch] = useState(false);

  const activeChat = chats[0];

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = messageInput.trim();
    if (!trimmed) return;
    setChats((currentChats) => {
      const [first, ...rest] = currentChats;
      if (!first) return currentChats;
      return [
        {
          ...first,
          messages: [
            ...first.messages,
            { id: crypto.randomUUID(), role: 'user', content: trimmed },
          ],
        },
        ...rest,
      ];
    });
    setMessageInput('');
  };

  const addTerminal = () => {
    setTerminalIds((ids) => {
      const nextId = Math.max(0, ...ids) + 1;
      setActiveTerminalId(nextId);
      return [...ids, nextId];
    });
  };

  const removeTerminal = (id: number) => {
    setTerminalIds((ids) => {
      const nextIds = ids.filter((terminalId) => terminalId !== id);
      if (activeTerminalId === id) {
        const fallback = nextIds[0] ?? 1;
        setActiveTerminalId(fallback);
      }
      return nextIds.length ? nextIds : [1];
    });
  };

  const terminalTabValue = activeTerminalId === 0 ? 'commands' : `terminal-${activeTerminalId}`;

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <header className="flex items-center justify-between p-3">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild>
            <Link href="/">
              <IconArrowLeft className="h-4 w-4" />
              Back
            </Link>
          </Button>
          <span className="text-xs text-muted-foreground">Task details</span>
          <div className="flex items-center gap-2">
            <div className="group flex items-center gap-2 rounded-md px-2 h-8 hover:bg-muted/40 cursor-default">
              <IconGitFork className="h-4 w-4 text-muted-foreground" />
              {isEditingBranch ? (
                <input
                  className="bg-background text-sm outline-none w-[160px] rounded-md border border-border/70 px-1"
                  value={branchName}
                  onChange={(event) => setBranchName(event.target.value)}
                  onBlur={() => setIsEditingBranch(false)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === 'Escape') {
                      setIsEditingBranch(false);
                    }
                  }}
                  autoFocus
                />
              ) : (
                <>
                  <span className="text-sm font-medium">{branchName}</span>
                  <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                    <button
                      type="button"
                      className="text-muted-foreground hover:text-foreground cursor-pointer"
                      onClick={() => setIsEditingBranch(true)}
                    >
                      <IconPencil className="h-3.5 w-3.5" />
                    </button>
                    <button
                      type="button"
                      className="text-muted-foreground hover:text-foreground cursor-pointer"
                      onClick={() => navigator.clipboard?.writeText(branchName)}
                    >
                      <IconCopy className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </>
              )}
            </div>
            <IconChevronRight className="h-4 w-4 text-muted-foreground" />
            <Select defaultValue="origin/main">
              <SelectTrigger className="w-[190px] h-8 cursor-pointer border border-transparent bg-transparent hover:bg-muted/40 data-[state=open]:bg-background data-[state=open]:border-border/70">
                <div className="flex items-center gap-2">
                  <IconGitBranch className="h-4 w-4 text-muted-foreground" />
                  <SelectValue placeholder="Base branch" />
                </div>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="origin/main">origin/main</SelectItem>
                <SelectItem value="origin/develop">origin/develop</SelectItem>
                <SelectItem value="origin/release">origin/release</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <CommitStatBadge label="2 ahead" tone="ahead" />
          <CommitStatBadge label="4 behind" tone="behind" />
          <LineStat added={855} removed={8} />
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" className="cursor-pointer">
            <IconBrandVscode className="h-4 w-4" />
            Editor
          </Button>
          <Button size="sm" variant="outline" className="cursor-pointer">
            <IconArrowDown className="h-4 w-4" />
            Pull
          </Button>
          <Button size="sm" variant="outline" className="cursor-pointer">
            <IconEye className="h-4 w-4" />
            Review
          </Button>
          <Select defaultValue="create-pr">
            <SelectTrigger className="w-[190px] h-8 cursor-pointer">
              <SelectValue placeholder="Create PR" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="create-pr">
                <span className="flex items-center gap-2">
                  <IconGitPullRequest className="h-4 w-4" />
                  Create PR
                </span>
              </SelectItem>
              <SelectItem value="create-pr-manual">
                <span className="flex items-center gap-2">
                  <IconPencil className="h-4 w-4" />
                  Create PR manually
                </span>
              </SelectItem>
              <SelectItem value="merge">
                <span className="flex items-center gap-2">
                  <IconGitMerge className="h-4 w-4" />
                  Merge
                </span>
              </SelectItem>
              <SelectItem value="rebase">
                <span className="flex items-center gap-2">
                  <IconGitBranch className="h-4 w-4" />
                  Rebase
                </span>
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
      </header>

      <div className="flex-1 min-h-0 px-4 pb-4">
        <ResizablePanelGroup direction="horizontal" className="h-full">
          <ResizablePanel defaultSize={75} minSize={55}>
            <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-r-0">
              <Tabs
                value={leftTab}
                onValueChange={(value) => setLeftTab(value as typeof leftTab)}
                className="flex-1 min-h-0"
              >
                <TabsList>
                  <TabsTrigger value="notes">Notes</TabsTrigger>
                  <TabsTrigger value="changes">All changes</TabsTrigger>
                  <TabsTrigger value="chat">Chat {activeChat ? `• ${activeChat.title}` : ''}</TabsTrigger>
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
                  <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
                    <pre className="text-xs leading-relaxed whitespace-pre-wrap text-foreground">{`diff --git a/apps/web/components/kanban-board.tsx b/apps/web/components/kanban-board.tsx
index 1234567..89abcde 100644
--- a/apps/web/components/kanban-board.tsx
+++ b/apps/web/components/kanban-board.tsx
@@ -1,5 +1,5 @@
-export function KanbanBoard() {
+export function KanbanBoard() {
 // ...`}</pre>
                  </div>
                </TabsContent>

                <TabsContent value="chat" className="mt-3 flex flex-col min-h-0 flex-1">
                  <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 space-y-3">
                    {(activeChat?.messages ?? []).map((message) => (
                      <div
                        key={message.id}
                        className={cn(
                          'max-w-[80%] rounded-lg px-3 py-2 text-sm leading-relaxed',
                          message.role === 'user'
                            ? 'ml-auto bg-primary text-primary-foreground'
                            : 'bg-muted text-foreground'
                        )}
                      >
                        <p className="text-[11px] uppercase tracking-wide mb-1 opacity-70">
                          {message.role === 'user' ? 'You' : 'Agent'}
                        </p>
                        <p>{message.content}</p>
                      </div>
                    ))}
                  </div>
                  <form onSubmit={handleSubmit} className="mt-3 flex flex-col gap-2">
                    <Textarea
                      value={messageInput}
                      onChange={(event) => setMessageInput(event.target.value)}
                      placeholder="Write to submit work to the agent..."
                      className="min-h-[90px] resize-none"
                    />
                    <div className="flex items-center justify-between gap-2">
                      <Select value={selectedAgent} onValueChange={setSelectedAgent}>
                        <SelectTrigger className="w-[160px]">
                          <SelectValue placeholder="Select agent" />
                        </SelectTrigger>
                        <SelectContent>
                          {AGENTS.map((agent) => (
                            <SelectItem key={agent.id} value={agent.id}>
                              {agent.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Button type="submit">Submit</Button>
                    </div>
                  </form>
                </TabsContent>
              </Tabs>
            </div>
          </ResizablePanel>
          <ResizableHandle className="w-px" />
          <ResizablePanel defaultSize={25} minSize={20}>
            {isBottomCollapsed ? (
              <div className="h-full min-h-0 flex flex-col gap-1">
                <div className="flex-1 min-h-0">
                  <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0">
                    <Tabs
                      value={topTab}
                      onValueChange={(value) => setTopTab(value as typeof topTab)}
                      className="flex-1 min-h-0"
                    >
                      <TabsList>
                        <TabsTrigger value="diff">Diff files</TabsTrigger>
                        <TabsTrigger value="files">All files</TabsTrigger>
                      </TabsList>
                      <TabsContent value="diff" className="mt-3 flex-1 min-h-0">
                        <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
                          <ul className="space-y-2">
                            {CHANGED_FILES.map((file) => {
                              const { folder, file: name } = splitPath(file.path);
                              return (
                                <li
                                  key={file.path}
                                  className="flex items-center justify-between gap-3 text-sm"
                                >
                                  <div className="min-w-0">
                                    <p className="truncate text-foreground">
                                      <span className="text-foreground/60">{folder}/</span>
                                      <span className="font-medium text-foreground">{name}</span>
                                    </p>
                                  </div>
                                  <div className="flex items-center gap-2">
                                    <LineStat added={file.plus} removed={file.minus} />
                                    <Badge className={badgeClass(file.status)}>{file.status}</Badge>
                                  </div>
                                </li>
                              );
                            })}
                          </ul>
                        </div>
                      </TabsContent>
                      <TabsContent value="files" className="mt-3 flex-1 min-h-0">
                        <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
                          <ul className="space-y-2 text-sm">
                            {ALL_FILES.map((file) => {
                              const { folder, file: name } = splitPath(file);
                              return (
                                <li key={file} className="truncate text-foreground">
                                  <span className="text-foreground/60">{folder}/</span>
                                  <span className="font-medium text-foreground">{name}</span>
                                </li>
                              );
                            })}
                          </ul>
                        </div>
                      </TabsContent>
                    </Tabs>
                  </div>
                </div>
                <div className="h-12 border border-border/70 rounded-lg bg-card flex items-center justify-between px-3 border-l-0">
                  <Tabs
                    value={terminalTabValue}
                    onValueChange={(value) => {
                      if (value === 'commands') {
                        setActiveTerminalId(0);
                        return;
                      }
                      const parsed = Number(value.replace('terminal-', ''));
                      if (!Number.isNaN(parsed)) {
                        setActiveTerminalId(parsed);
                      }
                    }}
                    className="flex-1 min-h-0"
                  >
                    <TabsList>
                      <TabsTrigger value="commands">Commands</TabsTrigger>
                      {terminalIds.map((id) => (
                        <TabsTrigger key={id} value={`terminal-${id}`}>
                          {id === 1 ? 'Terminal' : `Terminal ${id}`}
                        </TabsTrigger>
                      ))}
                      <TabsTrigger value="add" onClick={addTerminal}>
                        +
                      </TabsTrigger>
                    </TabsList>
                  </Tabs>
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground cursor-pointer"
                    onClick={() => setIsBottomCollapsed(false)}
                  >
                    <IconChevronUp className="h-4 w-4" />
                  </button>
                </div>
              </div>
            ) : (
              <ResizablePanelGroup direction="vertical" className="h-full">
                <ResizablePanel defaultSize={55} minSize={30}>
                  <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0">
                    <Tabs
                      value={topTab}
                      onValueChange={(value) => setTopTab(value as typeof topTab)}
                      className="flex-1 min-h-0"
                    >
                      <TabsList>
                        <TabsTrigger value="diff">Diff files</TabsTrigger>
                        <TabsTrigger value="files">All files</TabsTrigger>
                      </TabsList>
                      <TabsContent value="diff" className="mt-3 flex-1 min-h-0">
                        <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
                          <ul className="space-y-2">
                            {CHANGED_FILES.map((file) => {
                              const { folder, file: name } = splitPath(file.path);
                              return (
                                <li
                                  key={file.path}
                                  className="flex items-center justify-between gap-3 text-sm"
                                >
                                  <div className="min-w-0">
                                    <p className="truncate text-foreground">
                                      <span className="text-foreground/60">{folder}/</span>
                                      <span className="font-medium text-foreground">{name}</span>
                                    </p>
                                  </div>
                                  <div className="flex items-center gap-2">
                                    <LineStat added={file.plus} removed={file.minus} />
                                    <Badge className={badgeClass(file.status)}>{file.status}</Badge>
                                  </div>
                                </li>
                              );
                            })}
                          </ul>
                        </div>
                      </TabsContent>
                      <TabsContent value="files" className="mt-3 flex-1 min-h-0">
                        <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
                          <ul className="space-y-2 text-sm">
                            {ALL_FILES.map((file) => {
                              const { folder, file: name } = splitPath(file);
                              return (
                                <li key={file} className="truncate text-foreground">
                                  <span className="text-foreground/60">{folder}/</span>
                                  <span className="font-medium text-foreground">{name}</span>
                                </li>
                              );
                            })}
                          </ul>
                        </div>
                      </TabsContent>
                    </Tabs>
                  </div>
                </ResizablePanel>
                <ResizableHandle className="h-px" />
                <ResizablePanel defaultSize={45} minSize={20}>
                  <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0">
                    <Tabs
                      value={terminalTabValue}
                      onValueChange={(value) => {
                        if (value === 'commands') {
                          setActiveTerminalId(0);
                          return;
                        }
                        const parsed = Number(value.replace('terminal-', ''));
                        if (!Number.isNaN(parsed)) {
                          setActiveTerminalId(parsed);
                        }
                      }}
                      className="flex-1 min-h-0"
                    >
                      <div className="flex items-center justify-between mb-3">
                        <TabsList>
                          <TabsTrigger value="commands">Commands</TabsTrigger>
                          {terminalIds.map((id) => (
                            <div key={id} className="group flex items-center gap-1">
                              <TabsTrigger value={`terminal-${id}`}>
                                {id === 1 ? 'Terminal' : `Terminal ${id}`}
                              </TabsTrigger>
                              <button
                                type="button"
                                className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                                onClick={() => removeTerminal(id)}
                              >
                                <IconX className="h-3.5 w-3.5" />
                              </button>
                            </div>
                          ))}
                          <TabsTrigger value="add" onClick={addTerminal}>
                            +
                          </TabsTrigger>
                        </TabsList>
                        <button
                          type="button"
                          className="text-muted-foreground hover:text-foreground cursor-pointer"
                          onClick={() => setIsBottomCollapsed(true)}
                        >
                          <IconChevronDown className="h-4 w-4" />
                        </button>
                      </div>
                      <TabsContent value="commands" className="flex-1 min-h-0">
                        <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 space-y-3 h-full">
                          <div className="grid gap-2">
                            {COMMANDS.map((command) => (
                              <button
                                key={command.id}
                                type="button"
                                className="flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm text-left hover:bg-muted"
                              >
                                <span>{command.label}</span>
                                <Badge variant="secondary">Run</Badge>
                              </button>
                            ))}
                          </div>
                        </div>
                      </TabsContent>
                      {terminalIds.map((id) => (
                        <TabsContent key={id} value={`terminal-${id}`} className="flex-1 min-h-0">
                          <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 space-y-3 h-full">
                            <div className="rounded-md border border-border bg-black/90 text-green-200 font-mono text-xs p-3 space-y-2">
                              <p className="text-green-400">kan-dev@workspace:~$</p>
                              <p className="text-green-200">_</p>
                            </div>
                          </div>
                        </TabsContent>
                      ))}
                    </Tabs>
                  </div>
                </ResizablePanel>
              </ResizablePanelGroup>
            )}
          </ResizablePanel>
        </ResizablePanelGroup>
      </div>
    </div>
  );
}
