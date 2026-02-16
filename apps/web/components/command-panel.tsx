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

const ARCHIVED_STATES = new Set(['COMPLETED', 'CANCELLED', 'FAILED']);

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

export function CommandPanel() {
  const { open, setOpen } = useCommandPanelOpen();
  const commands = useCommands();
  const router = useRouter();
  const { toast } = useToast();
  const [mode, setMode] = useState<CommandPanelMode>('commands');
  const [search, setSearch] = useState('');
  const [taskResults, setTaskResults] = useState<Task[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const kanbanSteps = useAppStore((state) => state.kanban.steps);

  // Map step IDs to step names for display
  const stepMap = useMemo(() => {
    const map = new Map<string, { name: string; color: string }>();
    for (const step of kanbanSteps) {
      map.set(step.id, { name: step.title, color: step.color });
    }
    return map;
  }, [kanbanSteps]);

  // Toggle panel with Cmd+K, Cmd+P, or Cmd+Shift+P
  const openRef = useRef(open);
  openRef.current = open;
  const toggle = useCallback(() => setOpen(!openRef.current), [setOpen]);
  useKeyboardShortcut(SHORTCUTS.SEARCH, toggle);
  useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL, toggle);
  useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL_SHIFT, toggle);

  // Reset state when panel closes
  useEffect(() => {
    if (!open) {
      // Delay reset to avoid flash during close animation
      const t = setTimeout(() => {
        setMode('commands');
        setSearch('');
        setTaskResults([]);
      }, 200);
      return () => clearTimeout(t);
    }
  }, [open]);

  // Debounced task search with AbortController to prevent stale results
  const abortRef = useRef<AbortController | null>(null);
  useEffect(() => {
    if (mode !== 'search-tasks') return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    abortRef.current?.abort();

    if (!search.trim()) {
      setTaskResults([]);
      setIsSearching(false);
      return;
    }

    setIsSearching(true);
    debounceRef.current = setTimeout(async () => {
      if (!workspaceId) {
        setIsSearching(false);
        return;
      }
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const res = await listTasksByWorkspace(
          workspaceId,
          { query: search.trim(), page: 1, pageSize: 10 },
          { init: { signal: controller.signal } }
        );
        if (!controller.signal.aborted) {
          setTaskResults(res.tasks ?? []);
        }
      } catch {
        if (!controller.signal.aborted) {
          setTaskResults([]);
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsSearching(false);
        }
      }
    }, 300);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      abortRef.current?.abort();
    };
  }, [search, mode, workspaceId]);

  // Group commands by group field, sorted by best (lowest) priority in each group
  const grouped = useMemo(() => {
    const map = new Map<string, CommandItemType[]>();
    for (const cmd of commands) {
      const existing = map.get(cmd.group) ?? [];
      existing.push(cmd);
      map.set(cmd.group, existing);
    }
    // Sort groups by the minimum priority of their items
    const entries = Array.from(map.entries()).sort(([, a], [, b]) => {
      const minA = Math.min(...a.map((c) => c.priority ?? 100));
      const minB = Math.min(...b.map((c) => c.priority ?? 100));
      return minA - minB;
    });
    return entries;
  }, [commands]);

  const handleSelect = useCallback(
    (cmd: CommandItemType) => {
      if (cmd.enterMode) {
        setMode(cmd.enterMode);
        setSearch('');
        return;
      }
      if (cmd.action) {
        setOpen(false);
        cmd.action();
      }
    },
    [setOpen]
  );

  const handleTaskSelect = useCallback(
    (task: Task) => {
      setOpen(false);
      if (task.primary_session_id) {
        router.push(linkToSession(task.primary_session_id));
      } else {
        toast({ title: 'This task has no active session', variant: 'default' });
      }
    },
    [setOpen, router, toast]
  );

  // Handle backspace on empty input to go back to commands mode
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (mode !== 'commands' && e.key === 'Backspace' && !search) {
        e.preventDefault();
        setMode('commands');
        setSearch('');
      }
    },
    [mode, search]
  );

  return (
    <CommandDialog open={open} onOpenChange={setOpen} overlayClassName="supports-backdrop-filter:backdrop-blur-none!">
      <Command shouldFilter={mode === 'commands'} loop>
        <div className="flex items-center border-b border-border [&>[data-slot=command-input-wrapper]]:flex-1">
          {mode === 'search-tasks' && (
            <button
              onClick={() => { setMode('commands'); setSearch(''); }}
              className="shrink-0 pl-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
            >
              <span>←</span>
              <span>Tasks</span>
              <span className="text-muted-foreground/50">›</span>
            </button>
          )}
          <CommandInput
            placeholder={mode === 'commands' ? 'Type a command...' : 'Search for tasks...'}
            value={search}
            onValueChange={setSearch}
            onKeyDown={handleKeyDown}
          />
        </div>

        <CommandList>
          {mode === 'commands' && (
            <>
              <CommandEmpty>No commands found.</CommandEmpty>
              {search.trim() ? (
                // Flat list when searching — lets cmdk rank by relevance globally
                <CommandGroup>
                  {commands.map((cmd) => (
                    <CommandItemRow key={cmd.id} cmd={cmd} onSelect={handleSelect} />
                  ))}
                </CommandGroup>
              ) : (
                // Grouped by priority when browsing
                grouped.map(([group, items]) => (
                  <CommandGroup key={group} heading={group}>
                    {items.map((cmd) => (
                      <CommandItemRow key={cmd.id} cmd={cmd} onSelect={handleSelect} />
                    ))}
                  </CommandGroup>
                ))
              )}
            </>
          )}

          {mode === 'search-tasks' && (
            <>
              {isSearching && (
                <div className="flex items-center justify-center py-6">
                  <IconLoader2 className="size-4 animate-spin text-muted-foreground" />
                </div>
              )}
              {!isSearching && search.trim() && taskResults.length === 0 && (
                <CommandEmpty>No tasks found.</CommandEmpty>
              )}
              {!isSearching && !search.trim() && (
                <CommandEmpty>Type to search tasks...</CommandEmpty>
              )}
              {!isSearching && taskResults.length > 0 && (
                <CommandGroup heading="Results">
                  {taskResults.map((task) => {
                    const isArchived = ARCHIVED_STATES.has(task.state);
                    const step = stepMap.get(task.workflow_step_id);
                    return (
                      <CommandItem
                        key={task.id}
                        value={task.id}
                        onSelect={() => handleTaskSelect(task)}
                        className={isArchived ? 'opacity-60' : ''}
                      >
                        <div className="flex flex-col gap-0.5 min-w-0">
                          <div className="flex items-center gap-2">
                            <IconSearch className="size-3 shrink-0 text-muted-foreground" />
                            <span className="truncate font-medium">{task.title}</span>
                            {step && (
                              <Badge variant="secondary" className="text-[0.6rem] shrink-0">
                                {step.name}
                              </Badge>
                            )}
                            {isArchived && (
                              <Badge variant="outline" className="text-[0.6rem] shrink-0 opacity-70">
                                {task.state}
                              </Badge>
                            )}
                          </div>
                          {task.updated_at && (
                            <div className="flex items-center gap-1.5 text-[0.6rem] text-muted-foreground pl-5">
                              <span>{formatDistanceToNow(new Date(task.updated_at), { addSuffix: true })}</span>
                            </div>
                          )}
                        </div>
                      </CommandItem>
                    );
                  })}
                </CommandGroup>
              )}
            </>
          )}
        </CommandList>

        {/* Footer bar */}
        <div className="border-t border-border px-3 py-1.5 flex items-center gap-3 text-[0.6rem] text-muted-foreground">
          <KbdGroup>
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span>Navigate</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>↵</Kbd>
            <span>{mode === 'search-tasks' ? 'Open' : 'Select'}</span>
          </KbdGroup>
          {mode === 'search-tasks' && (
            <KbdGroup>
              <Kbd>⌫</Kbd>
              <span>Back</span>
            </KbdGroup>
          )}
          <KbdGroup>
            <Kbd>esc</Kbd>
            <span>Close</span>
          </KbdGroup>
        </div>
      </Command>
    </CommandDialog>
  );
}
