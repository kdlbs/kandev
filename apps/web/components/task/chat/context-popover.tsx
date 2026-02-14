'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { IconListCheck, IconFile, IconFolder, IconSearch, IconAt } from '@tabler/icons-react';
import { Popover, PopoverContent, PopoverTrigger } from '@kandev/ui/popover';
import { Checkbox } from '@kandev/ui/checkbox';
import { Input } from '@kandev/ui/input';
import { getWebSocketClient } from '@/lib/ws/connection';
import { searchWorkspaceFiles } from '@/lib/ws/workspace-files';
import { useCustomPrompts } from '@/hooks/domains/settings/use-custom-prompts';
import { isDirectory, getFileName } from '@/lib/utils/file-path';
import type { ContextFile } from '@/lib/state/context-files-store';

type ContextPopoverProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: React.ReactNode;
  sessionId: string | null;
  /** Whether plan is included as context (from context files store, not plan panel) */
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  onToggleFile: (file: ContextFile) => void;
};

const FILE_SEARCH_DEBOUNCE = 300;

const PLAN_CONTEXT_FILE: ContextFile = { path: 'plan:context', name: 'Plan', pinned: true };

export function ContextPopover({
  open,
  onOpenChange,
  trigger,
  sessionId,
  planContextEnabled,
  contextFiles,
  onToggleFile,
}: ContextPopoverProps) {
  const [query, setQuery] = useState('');
  const [fileResults, setFileResults] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const { prompts } = useCustomPrompts();

  // Focus input when popover opens
  /* eslint-disable react-hooks/set-state-in-effect -- resetting state on open/close is intentional */
  useEffect(() => {
    if (open) {
      requestAnimationFrame(() => inputRef.current?.focus());
    } else {
      setQuery('');
      setFileResults([]);
    }
  }, [open]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Debounced file search
  /* eslint-disable react-hooks/set-state-in-effect -- loading state sync is intentional for UX */
  useEffect(() => {
    if (!open || !sessionId) return;

    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }

    const delay = query === '' ? 0 : FILE_SEARCH_DEBOUNCE;
    setIsLoading(true);

    let cancelled = false;
    searchTimeoutRef.current = setTimeout(async () => {
      try {
        const client = getWebSocketClient();
        if (!client) {
          if (!cancelled) { setFileResults([]); setIsLoading(false); }
          return;
        }
        const response = await searchWorkspaceFiles(client, sessionId, query || '', 20);
        if (!cancelled) setFileResults(response.files || []);
      } catch {
        if (!cancelled) setFileResults([]);
      }
      if (!cancelled) setIsLoading(false);
    }, delay);

    return () => {
      cancelled = true;
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, [open, sessionId, query]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const handleToggleFile = useCallback(
    (filePath: string) => {
      const name = getFileName(filePath);
      onToggleFile({ path: filePath, name, pinned: true });
    },
    [onToggleFile]
  );

  const isFileSelected = useCallback(
    (path: string) => contextFiles.some((f) => f.path === path),
    [contextFiles]
  );

  // Filter prompts by query
  const filteredPrompts = query
    ? prompts.filter((p) => p.name.toLowerCase().includes(query.toLowerCase()))
    : prompts;

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        side="top"
        align="start"
        className="w-72 p-0 gap-1"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        {/* Header */}
        <div className="px-3 pt-3 pb-2 flex items-baseline gap-1.5">
          <p className="text-xs font-medium">Context</p>
          <p className="text-[10px] text-muted-foreground">· Select files and prompts to include</p>
        </div>

        {/* Search */}
        <div className="px-3 pb-2">
          <div className="relative">
            <IconSearch className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              ref={inputRef}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search files and prompts..."
              className="h-7 pl-7 text-xs"
            />
          </div>
        </div>

        {/* Items */}
        <div className="max-h-60 overflow-y-auto border-t border-border">
          {/* Plan item - always first, only when no search query */}
          {!query && (
            <div
              className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer"
              onClick={() => onToggleFile(PLAN_CONTEXT_FILE)}
            >
              <Checkbox
                checked={planContextEnabled}
                onCheckedChange={() => onToggleFile(PLAN_CONTEXT_FILE)}
                className="h-3.5 w-3.5"
              />
              <IconListCheck className="h-4 w-4 text-muted-foreground shrink-0" />
              <span className="text-xs flex-1 truncate">Plan</span>
              {planContextEnabled && (
                <span className="text-[9px] font-medium text-primary bg-primary/10 px-1.5 py-0.5 rounded">
                  ACTIVE
                </span>
              )}
            </div>
          )}

          {/* Prompts section */}
          {filteredPrompts.length > 0 && (
            <>
              {!query && (
                <div className="px-3 pt-2 pb-1">
                  <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">Prompts</p>
                </div>
              )}
              {filteredPrompts.map((prompt) => (
                <div
                  key={prompt.id}
                  className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer"
                  onClick={() => {
                    // Insert prompt content by simulating a mention select - for now just copy to clipboard or similar
                    // Prompts in context popover: clicking inserts the prompt name as a context file reference
                    // This toggles the prompt as a context file
                    onToggleFile({ path: `prompt:${prompt.id}`, name: prompt.name, pinned: true });
                  }}
                >
                  <Checkbox
                    checked={contextFiles.some((f) => f.path === `prompt:${prompt.id}`)}
                    onCheckedChange={() => onToggleFile({ path: `prompt:${prompt.id}`, name: prompt.name, pinned: true })}
                    className="h-3.5 w-3.5"
                  />
                  <IconAt className="h-4 w-4 text-muted-foreground shrink-0" />
                  <div className="flex-1 min-w-0">
                    <span className="text-xs truncate block">{prompt.name}</span>
                    {prompt.content && (
                      <span className="text-[10px] text-muted-foreground truncate block">
                        {prompt.content.length > 60 ? prompt.content.slice(0, 60) + '...' : prompt.content}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </>
          )}

          {/* Files section header */}
          {!query && fileResults.length > 0 && (
            <div className="px-3 pt-2 pb-1">
              <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">Files</p>
            </div>
          )}

          {/* File results */}
          {isLoading ? (
            <div className="px-3 py-3 text-center text-xs text-muted-foreground">Loading...</div>
          ) : fileResults.length === 0 && query && filteredPrompts.length === 0 ? (
            <div className="px-3 py-3 text-center text-xs text-muted-foreground">No results found</div>
          ) : (
            fileResults.map((filePath) => {
              const isDir = isDirectory(filePath);
              const name = getFileName(filePath);
              const parent = filePath.slice(0, filePath.length - name.length);
              return (
                <div
                  key={filePath}
                  className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted/50 cursor-pointer"
                  onClick={() => handleToggleFile(filePath)}
                >
                  <Checkbox
                    checked={isFileSelected(filePath)}
                    onCheckedChange={() => handleToggleFile(filePath)}
                    className="h-3.5 w-3.5"
                  />
                  {isDir ? (
                    <IconFolder className="h-4 w-4 text-muted-foreground shrink-0" />
                  ) : (
                    <IconFile className="h-4 w-4 text-muted-foreground shrink-0" />
                  )}
                  <div className="flex-1 min-w-0">
                    <span className="text-xs truncate block">{name}</span>
                    {parent && (
                      <span className="text-[10px] text-muted-foreground truncate block">{parent}</span>
                    )}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
