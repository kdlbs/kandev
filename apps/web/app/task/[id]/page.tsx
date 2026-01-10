'use client';

import { FormEvent, useMemo, useState } from 'react';
import Link from 'next/link';
import { DiffModeEnum, DiffView } from '@git-diff-view/react';
import '@git-diff-view/react/styles/diff-view.css';
import {
  IconArrowDown,
  IconArrowBackUp,
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
  IconBrain,
  IconLayoutColumns,
  IconLayoutRows,
  IconPencil,
  IconFile,
  IconFolder,
  IconListCheck,
  IconPaperclip,
  IconExternalLink,
  IconX,
} from '@tabler/icons-react';
import { useTheme } from 'next-themes';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarProvider,
  SidebarRail,
} from '@/components/ui/sidebar';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { CommitStatBadge, LineStat } from '@/components/diff-stat';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';

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

const DIFF_SAMPLES: Record<string, { diff: string; newContent: string }> = {
  'apps/web/components/kanban-board.tsx': {
    diff: [
      'diff --git a/apps/web/components/kanban-board.tsx b/apps/web/components/kanban-board.tsx',
      'index 7b45ad2..9c0f2ee 100644',
      '--- a/apps/web/components/kanban-board.tsx',
      '+++ b/apps/web/components/kanban-board.tsx',
      '@@ -14,6 +14,7 @@ export function KanbanBoard() {',
      '   const columns = useMemo(() => getColumns(view), [view]);',
      '   const tasks = useMemo(() => getTasks(), []);',
      '+  const hasAlerts = tasks.some((task) => task.priority === "high");',
      '   return (',
      '     <div className="kanban-board">',
      '       <BoardHeader />',
    ].join('\n'),
    newContent: [
      'export function KanbanBoard() {',
      '  const columns = useMemo(() => getColumns(view), [view]);',
      '  const tasks = useMemo(() => getTasks(), []);',
      '  const hasAlerts = tasks.some((task) => task.priority === "high");',
      '  return (',
      '    <div className="kanban-board">',
      '      <BoardHeader />',
      '    </div>',
      '  );',
      '}',
    ].join('\n'),
  },
  'apps/web/components/kanban-card.tsx': {
    diff: [
      'diff --git a/apps/web/components/kanban-card.tsx b/apps/web/components/kanban-card.tsx',
      'index a14d022..a0cc12f 100644',
      '--- a/apps/web/components/kanban-card.tsx',
      '+++ b/apps/web/components/kanban-card.tsx',
      '@@ -8,7 +8,8 @@ export function KanbanCard({ task }: KanbanCardProps) {',
      '   return (',
      '     <Card className="kanban-card">',
      '-      <h4 className="title">{task.title}</h4>',
      '+      <h4 className="title">{task.title}</h4>',
      '+      <span className="tag">{task.assignee}</span>',
      '       <p className="summary">{task.summary}</p>',
      '     </Card>',
      '   );',
    ].join('\n'),
    newContent: [
      'export function KanbanCard({ task }: KanbanCardProps) {',
      '  return (',
      '    <Card className="kanban-card">',
      '      <h4 className="title">{task.title}</h4>',
      '      <span className="tag">{task.assignee}</span>',
      '      <p className="summary">{task.summary}</p>',
      '    </Card>',
      '  );',
      '}',
    ].join('\n'),
  },
  'apps/web/components/task-create-dialog.tsx': {
    diff: [
      'diff --git a/apps/web/components/task-create-dialog.tsx b/apps/web/components/task-create-dialog.tsx',
      'new file mode 100644',
      'index 0000000..6c1a1f0',
      '--- /dev/null',
      '+++ b/apps/web/components/task-create-dialog.tsx',
      '@@ -0,0 +1,6 @@',
      '+export function TaskCreateDialog() {',
      '+  return (',
      '+    <Dialog>',
      '+      <DialogContent>Create task</DialogContent>',
      '+    </Dialog>',
      '+  );',
      '+}',
    ].join('\n'),
    newContent: [
      'export function TaskCreateDialog() {',
      '  return (',
      '    <Dialog>',
      '      <DialogContent>Create task</DialogContent>',
      '    </Dialog>',
      '  );',
      '}',
    ].join('\n'),
  },
  'apps/web/components/kanban-column.tsx': {
    diff: [
      'diff --git a/apps/web/components/kanban-column.tsx b/apps/web/components/kanban-column.tsx',
      'deleted file mode 100644',
      'index 9a9b7aa..0000000',
      '--- a/apps/web/components/kanban-column.tsx',
      '+++ /dev/null',
      '@@ -1,5 +0,0 @@',
      '-export function KanbanColumn() {',
      '-  return (',
      '-    <section className="kanban-column">Column</section>',
      '-  );',
      '-}',
    ].join('\n'),
    newContent: '',
  },
};

const COMMANDS = [
  { id: 'dev', label: 'npm run dev' },
  { id: 'lint', label: 'npm run lint' },
  { id: 'test', label: 'npm run test' },
];

const FILE_TREE = [
  [
    'app',
    ['api', ['hello', ['route.ts']], 'page.tsx', 'layout.tsx', ['blog', ['page.tsx']]],
  ],
  ['components', ['ui', 'button.tsx', 'card.tsx'], 'header.tsx', 'footer.tsx'],
  ['lib', ['util.ts']],
  ['public', 'favicon.ico', 'vercel.svg'],
  '.eslintrc.json',
  '.gitignore',
  'next.config.js',
  'tailwind.config.js',
  'package.json',
  'README.md',
];

type TreeItem = string | TreeItem[];

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

function Tree({ item }: { item: TreeItem }) {
  const [name, ...items] = Array.isArray(item) ? item : [item];

  if (!items.length) {
    return (
      <SidebarMenuButton className="data-[active=true]:bg-transparent">
        <IconFile className="h-4 w-4" />
        {name}
      </SidebarMenuButton>
    );
  }

  return (
    <SidebarMenuItem>
      <Collapsible
        className="group/collapsible [&[data-state=open]>button>svg:first-child]:rotate-90"
        defaultOpen={name === 'components' || name === 'ui'}
      >
        <CollapsibleTrigger asChild>
          <SidebarMenuButton>
            <IconChevronRight className="h-4 w-4 transition-transform" />
            <IconFolder className="h-4 w-4" />
            {name}
          </SidebarMenuButton>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <SidebarMenuSub>
            {items.map((subItem, index) => (
              <Tree key={index} item={subItem} />
            ))}
          </SidebarMenuSub>
        </CollapsibleContent>
      </Collapsible>
    </SidebarMenuItem>
  );
}

function buildDiffData(filePath: string) {
  const sample = DIFF_SAMPLES[filePath];
  if (!sample) {
    return {
      hunks: [
        [
          `diff --git a/${filePath} b/${filePath}`,
          'index 0000000..0000000 100644',
          `--- a/${filePath}`,
          `+++ b/${filePath}`,
          '@@ -1,1 +1,1 @@',
          '-',
          '+',
        ].join('\n'),
      ],
      oldFile: { fileName: filePath, fileLang: 'ts' },
      newFile: { fileName: filePath, fileLang: 'ts' },
    };
  }

  return {
    hunks: [sample.diff],
    oldFile: { fileName: filePath, fileLang: 'ts' },
    newFile: { fileName: filePath, fileLang: 'ts' },
  };
}

export default function TaskPage() {
  const defaultHorizontalLayout = getLocalStorage<[number, number]>(
    'task-layout-horizontal',
    [75, 25]
  );
  const defaultRightLayout = getLocalStorage<[number, number]>('task-layout-right', [55, 45]);
  const defaultDiffMode = getLocalStorage<'unified' | 'split'>(
    'task-diff-view-mode',
    'unified'
  );
  const [selectedAgent, setSelectedAgent] = useState(AGENTS[0].id);
  const [chats, setChats] = useState<ChatSession[]>(INITIAL_CHATS);
  const [leftTab, setLeftTab] = useState('chat');
  const [selectedDiffPath, setSelectedDiffPath] = useState<string | null>(null);
  const [messageInput, setMessageInput] = useState('');
  const [topTab, setTopTab] = useState<'diff' | 'files'>('diff');
  const [activeTerminalId, setActiveTerminalId] = useState(1);
  const [terminalIds, setTerminalIds] = useState([1]);
  const [notes, setNotes] = useState('');
  const [isBottomCollapsed, setIsBottomCollapsed] = useState(false);
  const [branchName, setBranchName] = useState('feature/agent-ui');
  const [isEditingBranch, setIsEditingBranch] = useState(false);
  const [diffViewMode, setDiffViewMode] = useState<'unified' | 'split'>(defaultDiffMode);
  const [planModeEnabled, setPlanModeEnabled] = useState(false);
  const { resolvedTheme } = useTheme();

  const activeChat = chats[0];
  const selectedDiffLabel = selectedDiffPath ?? 'All files';
  const diffTargets = useMemo(
    () => (selectedDiffPath ? [selectedDiffPath] : CHANGED_FILES.map((file) => file.path)),
    [selectedDiffPath]
  );
  const diffModeEnum = diffViewMode === 'split' ? DiffModeEnum.Split : DiffModeEnum.Unified;
  const diffTheme = resolvedTheme === 'dark' ? 'dark' : 'light';
  const isSingleDiffSelected = Boolean(selectedDiffPath && DIFF_SAMPLES[selectedDiffPath]);
  const selectedDiffContent = selectedDiffPath
    ? DIFF_SAMPLES[selectedDiffPath]?.newContent ?? ''
    : '';

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
  const topFilesPanel = (
    <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0">
      <Tabs
        value={topTab}
        onValueChange={(value) => setTopTab(value as typeof topTab)}
        className="flex-1 min-h-0"
      >
                <TabsList>
                  <TabsTrigger value="diff" className="cursor-pointer">
                    Diff files
                  </TabsTrigger>
                  <TabsTrigger value="files" className="cursor-pointer">
                    All files
                  </TabsTrigger>
                </TabsList>
        <TabsContent value="diff" className="mt-3 flex-1 min-h-0">
          <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
            <ul className="space-y-2">
              {CHANGED_FILES.map((file) => {
                const { folder, file: name } = splitPath(file.path);
                return (
                  <li
                    key={file.path}
                    className="group flex items-center justify-between gap-3 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
                    onClick={() => {
                      setSelectedDiffPath(file.path);
                      setLeftTab('changes');
                    }}
                  >
                    <button type="button" className="min-w-0 text-left cursor-pointer">
                      <p className="truncate text-foreground">
                        <span className="text-foreground/60">{folder}/</span>
                        <span className="font-medium text-foreground">{name}</span>
                      </p>
                    </button>
                    <div className="flex items-center gap-2">
                      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              onClick={(event) => {
                                event.stopPropagation();
                              }}
                            >
                              <IconArrowBackUp className="h-3.5 w-3.5" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>Discard changes</TooltipContent>
                        </Tooltip>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              onClick={(event) => {
                                event.stopPropagation();
                              }}
                            >
                              <IconExternalLink className="h-3.5 w-3.5" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>Open in editor</TooltipContent>
                        </Tooltip>
                      </div>
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
          <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background h-full">
            <SidebarProvider className="h-full w-full" style={{ "--sidebar-width": "100%" }}>
              <Sidebar collapsible="none" className="h-full w-full">
                <SidebarContent>
                  <SidebarGroup>
                    <SidebarGroupContent>
                      <SidebarMenu>
                        {FILE_TREE.map((item, index) => (
                          <Tree key={index} item={item} />
                        ))}
                      </SidebarMenu>
                    </SidebarGroupContent>
                  </SidebarGroup>
                </SidebarContent>
                <SidebarRail />
              </Sidebar>
            </SidebarProvider>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );

  return (
    <TooltipProvider>
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
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          className="text-muted-foreground hover:text-foreground cursor-pointer"
                          onClick={() => setIsEditingBranch(true)}
                        >
                          <IconPencil className="h-3.5 w-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>Edit branch name</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          className="text-muted-foreground hover:text-foreground cursor-pointer"
                          onClick={() => navigator.clipboard?.writeText(branchName)}
                        >
                          <IconCopy className="h-3.5 w-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>Copy branch name</TooltipContent>
                    </Tooltip>
                  </div>
                </>
              )}
            </div>
            <IconChevronRight className="h-4 w-4 text-muted-foreground" />
            <Select defaultValue="origin/main">
              <Tooltip>
                <TooltipTrigger asChild>
                  <SelectTrigger className="w-[190px] h-8 cursor-pointer border border-transparent bg-transparent hover:bg-muted/40 data-[state=open]:bg-background data-[state=open]:border-border/70">
                    <SelectValue placeholder="Base branch" />
                  </SelectTrigger>
                </TooltipTrigger>
                <TooltipContent>Change base branch</TooltipContent>
              </Tooltip>
              <SelectContent>
                <SelectItem value="origin/main">origin/main</SelectItem>
                <SelectItem value="origin/develop">origin/develop</SelectItem>
                <SelectItem value="origin/release">origin/release</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-1">
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-default">
                  <CommitStatBadge label="2 ahead" tone="ahead" />
                </span>
              </TooltipTrigger>
              <TooltipContent>Commits ahead of base</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-default">
                  <CommitStatBadge label="4 behind" tone="behind" />
                </span>
              </TooltipTrigger>
              <TooltipContent>Commits behind base</TooltipContent>
            </Tooltip>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-default">
                <LineStat added={855} removed={8} />
              </span>
            </TooltipTrigger>
            <TooltipContent>Lines changed</TooltipContent>
          </Tooltip>
        </div>
        <div className="flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="sm" variant="outline" className="cursor-pointer">
                <IconBrandVscode className="h-4 w-4" />
                Editor
              </Button>
            </TooltipTrigger>
            <TooltipContent>Open in editor</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="sm" variant="outline" className="cursor-pointer">
                <IconArrowDown className="h-4 w-4" />
                Pull
              </Button>
            </TooltipTrigger>
            <TooltipContent>Pull from remote</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="sm" variant="outline" className="cursor-pointer">
                <IconEye className="h-4 w-4" />
                Review
              </Button>
            </TooltipTrigger>
            <TooltipContent>Open review</TooltipContent>
          </Tooltip>
          <div className="inline-flex rounded-md border border-border overflow-hidden">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="sm" variant="outline" className="rounded-none border-0 cursor-pointer">
                  <IconGitPullRequest className="h-4 w-4" />
                  Create PR
                </Button>
              </TooltipTrigger>
              <TooltipContent>Create PR</TooltipContent>
            </Tooltip>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size="sm"
                  variant="outline"
                  className="rounded-none border-0 px-2 cursor-pointer"
                >
                  <IconChevronDown className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem>
                  <IconPencil className="h-4 w-4" />
                  Create PR manually
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <IconGitMerge className="h-4 w-4" />
                  Merge
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <IconGitBranch className="h-4 w-4" />
                  Rebase
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </header>

      <div className="flex-1 min-h-0 px-4 pb-4">
        <ResizablePanelGroup
          direction="horizontal"
          className="h-full"
          onLayout={(sizes) => setLocalStorage('task-layout-horizontal', sizes)}
        >
          <ResizablePanel defaultSize={defaultHorizontalLayout[0]} minSize={55}>
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
                  <div className="flex flex-col gap-2 h-full">
                    <div className="flex items-center justify-between gap-3">
                      <Badge variant="secondary" className="rounded-full text-xs">
                        {selectedDiffLabel}
                      </Badge>
                      <div className="flex items-center gap-1.5">
                        {selectedDiffPath && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="h-7 px-2 text-xs cursor-pointer"
                                onClick={() => setSelectedDiffPath(null)}
                              >
                                All changes
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>Show all changes</TooltipContent>
                          </Tooltip>
                        )}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="h-7 px-2 text-xs cursor-pointer"
                                disabled={!isSingleDiffSelected}
                                onClick={async () => {
                                  if (!isSingleDiffSelected) return;
                                  await navigator.clipboard.writeText(selectedDiffContent);
                                }}
                              >
                                <IconCopy className="h-3.5 w-3.5" />
                              </Button>
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>Copy file contents</TooltipContent>
                        </Tooltip>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="h-7 px-2 text-xs cursor-pointer"
                                disabled={!isSingleDiffSelected}
                              >
                                <IconArrowBackUp className="h-3.5 w-3.5" />
                              </Button>
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>Discard changes</TooltipContent>
                        </Tooltip>
                        <div className="inline-flex rounded-md border border-border overflow-hidden">
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  className={cn(
                                    'h-7 px-2 text-xs rounded-none cursor-pointer',
                                    diffViewMode === 'unified' && 'bg-muted'
                                  )}
                                  onClick={() => {
                                    setDiffViewMode('unified');
                                    setLocalStorage('task-diff-view-mode', 'unified');
                                  }}
                                >
                                  <IconLayoutRows className="h-3.5 w-3.5" />
                                </Button>
                              </span>
                            </TooltipTrigger>
                            <TooltipContent>Unified view</TooltipContent>
                          </Tooltip>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  className={cn(
                                    'h-7 px-2 text-xs rounded-none cursor-pointer',
                                    diffViewMode === 'split' && 'bg-muted'
                                  )}
                                  onClick={() => {
                                    setDiffViewMode('split');
                                    setLocalStorage('task-diff-view-mode', 'split');
                                  }}
                                >
                                  <IconLayoutColumns className="h-3.5 w-3.5" />
                                </Button>
                              </span>
                            </TooltipTrigger>
                            <TooltipContent>Split view</TooltipContent>
                          </Tooltip>
                        </div>
                      </div>
                    </div>
                    <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
                      <div className="space-y-4">
                        {diffTargets.map((path) => (
                          <div key={path} className="space-y-2">
                            {!selectedDiffPath && (
                              <div className="flex items-center justify-between">
                                <Badge variant="secondary" className="rounded-full text-xs">
                                  {path}
                                </Badge>
                              </div>
                            )}
                            <DiffView
                              data={buildDiffData(path)}
                              diffViewMode={diffModeEnum}
                              diffViewTheme={diffTheme}
                            />
                          </div>
                        ))}
                      </div>
                    </div>
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
                      className={cn(
                        'min-h-[90px] resize-none',
                        planModeEnabled &&
                          'border-dashed border-primary/60 !bg-primary/20 dark:!bg-primary/20 shadow-[inset_0_0_0_1px_rgba(59,130,246,0.35)]'
                      )}
                    />
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <Select value={selectedAgent} onValueChange={setSelectedAgent}>
                          <SelectTrigger className="w-[160px] cursor-pointer">
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
                        <DropdownMenu>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <DropdownMenuTrigger asChild>
                                <Button
                                  type="button"
                                  variant="outline"
                                  size="icon"
                                  className="h-9 w-9 cursor-pointer"
                                >
                                  <IconBrain className="h-4 w-4" />
                                </Button>
                              </DropdownMenuTrigger>
                            </TooltipTrigger>
                            <TooltipContent>Thinking level</TooltipContent>
                          </Tooltip>
                          <DropdownMenuContent align="start" side="top">
                            <DropdownMenuItem>High</DropdownMenuItem>
                            <DropdownMenuItem>Medium</DropdownMenuItem>
                            <DropdownMenuItem>Low</DropdownMenuItem>
                            <DropdownMenuItem>Off</DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <div className="flex items-center gap-2">
                              <Button
                                type="button"
                                variant="outline"
                                size="icon"
                                className={cn(
                                  'h-9 w-9 cursor-pointer',
                                  planModeEnabled &&
                                    'bg-primary/15 text-primary border-primary/40 shadow-[0_0_0_1px_rgba(59,130,246,0.35)]'
                                )}
                                onClick={() => setPlanModeEnabled((value) => !value)}
                              >
                                <IconListCheck className="h-4 w-4" />
                              </Button>
                              {planModeEnabled && (
                                <span className="text-xs font-medium text-primary">
                                  Plan mode active
                                </span>
                              )}
                            </div>
                          </TooltipTrigger>
                          <TooltipContent>Toggle plan mode</TooltipContent>
                        </Tooltip>
                      </div>
                      <div className="flex items-center gap-2">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              type="button"
                              variant="outline"
                              size="icon"
                              className="h-9 w-9 cursor-pointer"
                            >
                              <IconPaperclip className="h-4 w-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>Add attachments</TooltipContent>
                        </Tooltip>
                        <Button type="submit">Submit</Button>
                      </div>
                    </div>
                  </form>
                </TabsContent>
              </Tabs>
            </div>
          </ResizablePanel>
          <ResizableHandle className="w-px" />
          <ResizablePanel defaultSize={defaultHorizontalLayout[1]} minSize={20}>
            {isBottomCollapsed ? (
              <div className="h-full min-h-0 flex flex-col gap-1">
                <div className="flex-1 min-h-0">{topFilesPanel}</div>
                <div className="h-12 border border-border/70 rounded-lg bg-card flex items-center justify-between px-3 border-l-0 mt-[2px]">
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
                      <TabsTrigger value="commands" className="cursor-pointer">
                        Commands
                      </TabsTrigger>
                      {terminalIds.map((id) => (
                        <TabsTrigger key={id} value={`terminal-${id}`} className="cursor-pointer">
                          {id === 1 ? 'Terminal' : `Terminal ${id}`}
                        </TabsTrigger>
                      ))}
                      <TabsTrigger value="add" onClick={addTerminal} className="cursor-pointer">
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
              <ResizablePanelGroup
                direction="vertical"
                className="h-full"
                onLayout={(sizes) => setLocalStorage('task-layout-right', sizes)}
              >
                <ResizablePanel defaultSize={defaultRightLayout[0]} minSize={30}>
                  {topFilesPanel}
                </ResizablePanel>
                <ResizableHandle className="h-px" />
                <ResizablePanel defaultSize={defaultRightLayout[1]} minSize={20}>
                  <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0 mt-[5px]">
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
                          <TabsTrigger value="commands" className="cursor-pointer">
                            Commands
                          </TabsTrigger>
                          {terminalIds.map((id) => (
                            <TabsTrigger
                              key={id}
                              value={`terminal-${id}`}
                              className="group flex items-center gap-1 pr-1 cursor-pointer"
                            >
                              {id === 1 ? 'Terminal' : `Terminal ${id}`}
                              {terminalIds.length > 1 && (
                                <span
                                  role="button"
                                  tabIndex={-1}
                                  className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                                  onClick={(event) => {
                                    event.preventDefault();
                                    event.stopPropagation();
                                    removeTerminal(id);
                                  }}
                                >
                                  <IconX className="h-3.5 w-3.5" />
                                </span>
                              )}
                            </TabsTrigger>
                          ))}
                          <TabsTrigger value="add" onClick={addTerminal} className="cursor-pointer">
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
    </TooltipProvider>
  );
}
