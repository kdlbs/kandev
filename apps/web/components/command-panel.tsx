'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { formatDistanceToNow } from 'date-fns';
import {
  IconArrowRight,
  IconLoader2,
  IconSearch,
} from '@tabler/icons-react';
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from '@kandev/ui/command';
import { Kbd, KbdGroup } from '@kandev/ui/kbd';
import { Badge } from '@kandev/ui/badge';
import { useCommands, useCommandPanelOpen } from '@/lib/commands/command-registry';
import type { CommandPanelMode, CommandItem as CommandItemType } from '@/lib/commands/types';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { useKeyboardShortcut } from '@/hooks/use-keyboard-shortcut';
import { formatShortcut } from '@/lib/keyboard/utils';
import { useAppStore } from '@/components/state-provider';
import { useToast } from '@/components/toast-provider';
import { listTasksByWorkspace } from '@/lib/api';
import { linkToSession } from '@/lib/links';
import type { Task } from '@/lib/types/http';
import { getWebSocketClient } from '@/lib/ws/connection';
import { searchWorkspaceFiles } from '@/lib/ws/workspace-files';
import { FileIcon } from '@/components/ui/file-icon';
import { useDockviewStore } from '@/lib/state/dockview-store';

const ARCHIVED_STATES = new Set(['COMPLETED', 'CANCELLED', 'FAILED']);
const MODE_SEARCH_TASKS: CommandPanelMode = 'search-tasks';

function getFileName(filePath: string) {
  return filePath.split('/').pop() ?? filePath;
}

function CommandItemRow({ cmd, onSelect }: { cmd: CommandItemType; onSelect: (cmd: CommandItemType) => void }) {
  return (
    <CommandItem
      key={cmd.id}
      value={cmd.id + ' ' + cmd.label + ' ' + (cmd.keywords?.join(' ') ?? '')}
      onSelect={() => onSelect(cmd)}
    >
      {cmd.icon && <span className="text-muted-foreground">{cmd.icon}</span>}
      <span>{cmd.label}</span>
      {cmd.shortcut && <CommandShortcut>{formatShortcut(cmd.shortcut)}</CommandShortcut>}
      {cmd.enterMode && (
        <span className="ml-auto text-muted-foreground">
          <IconArrowRight className="size-3" />
        </span>
      )}
    </CommandItem>
  );
}

type TaskResultItemProps = { task: Task; stepMap: Map<string, { name: string; color: string }>; onSelect: (task: Task) => void };

function TaskResultItem({ task, stepMap, onSelect }: TaskResultItemProps) {
  const isArchived = ARCHIVED_STATES.has(task.state);
  const step = stepMap.get(task.workflow_step_id);
  return (
    <CommandItem key={task.id} value={task.id} onSelect={() => onSelect(task)} className={isArchived ? 'opacity-60' : ''}>
      <div className="flex flex-col gap-0.5 min-w-0">
        <div className="flex items-center gap-2">
          <IconSearch className="size-3 shrink-0 text-muted-foreground" />
          <span className="truncate font-medium">{task.title}</span>
          {step && <Badge variant="secondary" className="text-[0.6rem] shrink-0">{step.name}</Badge>}
          {isArchived && <Badge variant="outline" className="text-[0.6rem] shrink-0 opacity-70">{task.state}</Badge>}
        </div>
        {task.updated_at && (
          <div className="flex items-center gap-1.5 text-[0.6rem] text-muted-foreground pl-5">
            <span>{formatDistanceToNow(new Date(task.updated_at), { addSuffix: true })}</span>
          </div>
        )}
      </div>
    </CommandItem>
  );
}

type CommandsListContentProps = {
  commands: CommandItemType[];
  grouped: [string, CommandItemType[]][];
  search: string;
  onSelect: (cmd: CommandItemType) => void;
  hasFileResults: boolean;
};

function CommandsListContent({ commands, grouped, search, onSelect, hasFileResults }: CommandsListContentProps) {
  return (
    <>
      {!hasFileResults && <CommandEmpty>No commands found.</CommandEmpty>}
      {search.trim() ? (
        <CommandGroup>
          {commands.map((cmd) => <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />)}
        </CommandGroup>
      ) : (
        grouped.map(([group, items]) => (
          <CommandGroup key={group} heading={group}>
            {items.map((cmd) => <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />)}
          </CommandGroup>
        ))
      )}
    </>
  );
}

type SearchTasksContentProps = { isSearching: boolean; search: string; taskResults: Task[]; stepMap: Map<string, { name: string; color: string }>; onTaskSelect: (task: Task) => void };

function SearchTasksContent({ isSearching, search, taskResults, stepMap, onTaskSelect }: SearchTasksContentProps) {
  if (isSearching) return <div className="flex items-center justify-center py-6"><IconLoader2 className="size-4 animate-spin text-muted-foreground" /></div>;
  if (search.trim() && taskResults.length === 0) return <CommandEmpty>No tasks found.</CommandEmpty>;
  if (!search.trim()) return <CommandEmpty>Type to search tasks...</CommandEmpty>;
  return (
    <CommandGroup heading="Results">
      {taskResults.map((task) => <TaskResultItem key={task.id} task={task} stepMap={stepMap} onSelect={onTaskSelect} />)}
    </CommandGroup>
  );
}

type FileSearchResultsProps = { files: string[]; isSearching: boolean; onSelect: (path: string) => void };

function FileSearchResults({ files, isSearching, onSelect }: FileSearchResultsProps) {
  if (isSearching && files.length === 0) {
    return (
      <CommandGroup heading="Files" forceMount>
        <div className="flex items-center justify-center py-3">
          <IconLoader2 className="size-3.5 animate-spin text-muted-foreground" />
        </div>
      </CommandGroup>
    );
  }
  if (files.length === 0) return null;
  return (
    <CommandGroup heading="Files" forceMount>
      {files.map((filePath) => {
        const fileName = getFileName(filePath);
        const lastSlash = filePath.lastIndexOf('/');
        const dir = lastSlash > 0 ? filePath.slice(0, lastSlash) : '';
        return (
          <CommandItem key={filePath} value={`__file:${filePath}`} onSelect={() => onSelect(filePath)} forceMount>
            <FileIcon fileName={fileName} className="shrink-0" />
            <span className="font-medium truncate">{fileName}</span>
            {dir && <span className="text-muted-foreground text-xs truncate ml-1">{dir}</span>}
          </CommandItem>
        );
      })}
    </CommandGroup>
  );
}

function getInputPlaceholder(mode: CommandPanelMode, inputCommand: CommandItemType | null) {
  if (mode === 'input') return inputCommand?.inputPlaceholder ?? 'Enter value...';
  if (mode === MODE_SEARCH_TASKS) return 'Search for tasks...';
  return 'Type a command...';
}

function getEnterLabel(mode: CommandPanelMode) {
  if (mode === 'input') return 'Confirm';
  if (mode === MODE_SEARCH_TASKS) return 'Open';
  return 'Select';
}

function useCommandPanelState() {
  const [mode, setMode] = useState<CommandPanelMode>('commands');
  const [search, setSearch] = useState('');
  const [inputCommand, setInputCommand] = useState<CommandItemType | null>(null);
  const [taskResults, setTaskResults] = useState<Task[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [fileResults, setFileResults] = useState<string[]>([]);
  const [isSearchingFiles, setIsSearchingFiles] = useState(false);
  const [selectedValue, setSelectedValue] = useState('');
  return { mode, setMode, search, setSearch, inputCommand, setInputCommand, taskResults, setTaskResults, isSearching, setIsSearching, fileResults, setFileResults, isSearchingFiles, setIsSearchingFiles, selectedValue, setSelectedValue };
}

function useCommandPanelEffects(
  open: boolean,
  state: ReturnType<typeof useCommandPanelState>,
  workspaceId: string | null,
  activeSessionId: string | null,
) {
  const { mode, search, setMode, setSearch, setInputCommand, setTaskResults, setIsSearching, setFileResults, setIsSearchingFiles, setSelectedValue } = state;
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const fileDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!open) {
      const t = setTimeout(() => { setMode('commands'); setSearch(''); setInputCommand(null); setTaskResults([]); setFileResults([]); setSelectedValue(''); }, 200);
      return () => clearTimeout(t);
    }
  }, [open, setMode, setSearch, setInputCommand, setTaskResults, setFileResults, setSelectedValue]);

  useEffect(() => {
    if (mode !== MODE_SEARCH_TASKS) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    abortRef.current?.abort();
    if (!search.trim()) { setTaskResults([]); setIsSearching(false); return; }
    setIsSearching(true);
    debounceRef.current = setTimeout(async () => {
      if (!workspaceId) { setIsSearching(false); return; }
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const res = await listTasksByWorkspace(workspaceId, { query: search.trim(), page: 1, pageSize: 10 }, { init: { signal: controller.signal } });
        if (!controller.signal.aborted) setTaskResults(res.tasks ?? []);
      } catch {
        if (!controller.signal.aborted) setTaskResults([]);
      } finally {
        if (!controller.signal.aborted) setIsSearching(false);
      }
    }, 300);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); abortRef.current?.abort(); };
  }, [search, mode, workspaceId, setTaskResults, setIsSearching]);

  useEffect(() => {
    if (mode !== 'commands' || !search.trim() || !activeSessionId) {
      setFileResults([]); setIsSearchingFiles(false); return;
    }
    setIsSearchingFiles(true);
    if (fileDebounceRef.current) clearTimeout(fileDebounceRef.current);
    let cancelled = false;
    fileDebounceRef.current = setTimeout(async () => {
      const client = getWebSocketClient();
      if (!client || cancelled) { if (!cancelled) setIsSearchingFiles(false); return; }
      try {
        const res = await searchWorkspaceFiles(client, activeSessionId, search.trim(), 10);
        if (!cancelled) {
          const files = res.files ?? [];
          setFileResults(files);
          if (files.length > 0) setSelectedValue(`__file:${files[0]}`);
        }
      } catch {
        if (!cancelled) setFileResults([]);
      } finally {
        if (!cancelled) setIsSearchingFiles(false);
      }
    }, 250);
    return () => { cancelled = true; if (fileDebounceRef.current) clearTimeout(fileDebounceRef.current); };
  }, [search, mode, activeSessionId, setFileResults, setIsSearchingFiles, setSelectedValue]);
}

export function CommandPanel() {
  const { open, setOpen } = useCommandPanelOpen();
  const commands = useCommands();
  const router = useRouter();
  const { toast } = useToast();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const kanbanSteps = useAppStore((state) => state.kanban.steps);

  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);

  const state = useCommandPanelState();
  const { mode, setMode, search, setSearch, inputCommand, setInputCommand, taskResults, isSearching, fileResults, isSearchingFiles, selectedValue, setSelectedValue } = state;

  useCommandPanelEffects(open, state, workspaceId, activeSessionId);

  const stepMap = useMemo(() => {
    const map = new Map<string, { name: string; color: string }>();
    for (const step of kanbanSteps) map.set(step.id, { name: step.title, color: step.color });
    return map;
  }, [kanbanSteps]);

  const openRef = useRef(open);
  useEffect(() => { openRef.current = open; }, [open]);
  const toggle = useCallback(() => setOpen(!openRef.current), [setOpen]);
  useKeyboardShortcut(SHORTCUTS.SEARCH, toggle);
  useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL, toggle);
  useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL_SHIFT, toggle);

  const grouped = useMemo(() => {
    const map = new Map<string, CommandItemType[]>();
    for (const cmd of commands) { const existing = map.get(cmd.group) ?? []; existing.push(cmd); map.set(cmd.group, existing); }
    return Array.from(map.entries()).sort(([, a], [, b]) => Math.min(...a.map((c) => c.priority ?? 100)) - Math.min(...b.map((c) => c.priority ?? 100)));
  }, [commands]);

  const handleSelect = useCallback((cmd: CommandItemType) => {
    if (cmd.enterMode) { if (cmd.enterMode === 'input') setInputCommand(cmd); setMode(cmd.enterMode); setSearch(''); return; }
    if (cmd.action) { setOpen(false); cmd.action(); }
  }, [setOpen, setMode, setSearch, setInputCommand]);

  const handleTaskSelect = useCallback((task: Task) => {
    setOpen(false);
    if (task.primary_session_id) { router.push(linkToSession(task.primary_session_id)); }
    else { toast({ title: 'This task has no active session', variant: 'default' }); }
  }, [setOpen, router, toast]);

  const handleFileSelect = useCallback((filePath: string) => {
    setOpen(false);
    useDockviewStore.getState().addFileEditorPanel(filePath, getFileName(filePath));
  }, [setOpen]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (mode === 'input' && e.key === 'Enter' && search.trim() && inputCommand?.onInputSubmit) {
      e.preventDefault(); setOpen(false); inputCommand.onInputSubmit(search.trim()); return;
    }
    if (mode !== 'commands' && e.key === 'Backspace' && !search) {
      e.preventDefault(); setMode('commands'); setSearch(''); setInputCommand(null);
    }
  }, [mode, search, inputCommand, setOpen, setMode, setSearch, setInputCommand]);

  const goBack = useCallback(() => { setMode('commands'); setSearch(''); setInputCommand(null); }, [setMode, setSearch, setInputCommand]);

  return (
    <CommandDialog open={open} onOpenChange={setOpen} overlayClassName="supports-backdrop-filter:backdrop-blur-none!">
      <Command shouldFilter={mode === 'commands'} loop value={selectedValue} onValueChange={setSelectedValue}>
        <div className="flex items-center border-b border-border [&>[data-slot=command-input-wrapper]]:flex-1">
          {mode !== 'commands' && (
            <button onClick={goBack} className="shrink-0 pl-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
              <span>←</span>
              <span>{mode === 'input' ? inputCommand?.label : 'Tasks'}</span>
              <span className="text-muted-foreground/50">›</span>
            </button>
          )}
          <CommandInput placeholder={getInputPlaceholder(mode, inputCommand)} value={search} onValueChange={setSearch} onKeyDown={handleKeyDown} />
        </div>
        <CommandList>
          {mode === 'commands' && (
            <>
              {search.trim() && <FileSearchResults files={fileResults} isSearching={isSearchingFiles} onSelect={handleFileSelect} />}
              <CommandsListContent commands={commands} grouped={grouped} search={search} onSelect={handleSelect} hasFileResults={fileResults.length > 0 || isSearchingFiles} />
            </>
          )}
          {mode === MODE_SEARCH_TASKS && <SearchTasksContent isSearching={isSearching} search={search} taskResults={taskResults} stepMap={stepMap} onTaskSelect={handleTaskSelect} />}
          {mode === 'input' && (!search.trim() ? <CommandEmpty>{inputCommand?.inputPlaceholder ?? 'Enter a value...'}</CommandEmpty> : <CommandEmpty>Press Enter to confirm</CommandEmpty>)}
        </CommandList>
        <div className="border-t border-border px-3 py-1.5 flex items-center gap-3 text-[0.6rem] text-muted-foreground">
          {mode === 'commands' && <KbdGroup><Kbd>↑</Kbd><Kbd>↓</Kbd><span>Navigate</span></KbdGroup>}
          <KbdGroup><Kbd>↵</Kbd><span>{getEnterLabel(mode)}</span></KbdGroup>
          {mode !== 'commands' && <KbdGroup><Kbd>⌫</Kbd><span>Back</span></KbdGroup>}
          <KbdGroup><Kbd>esc</Kbd><span>Close</span></KbdGroup>
        </div>
      </Command>
    </CommandDialog>
  );
}
