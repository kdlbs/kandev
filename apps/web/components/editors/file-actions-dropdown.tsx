'use client';

import { useMemo, useCallback } from 'react';
import {
  IconExternalLink,
  IconCopy,
  IconFolderShare,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { useEditors } from '@/hooks/domains/settings/use-editors';
import { useOpenSessionInEditor } from '@/hooks/use-open-session-in-editor';
import { useOpenSessionFolder } from '@/hooks/use-open-session-folder';
import { useAppStore } from '@/components/state-provider';
import type { EditorOption } from '@/lib/types/http';

type FileActionsDropdownProps = {
  /** File path to open / copy */
  filePath: string;
  /** Session ID override — defaults to activeSessionId from store */
  sessionId?: string;
  /** Button size variant */
  size?: 'sm' | 'xs';
  /** Optional toast callback after copy */
  onCopied?: () => void;
};

export function FileActionsDropdown({
  filePath,
  sessionId: sessionIdProp,
  size = 'xs',
  onCopied,
}: FileActionsDropdownProps) {
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = sessionIdProp ?? storeSessionId ?? null;
  const worktreePath = useAppStore((state) => {
    if (!sessionId) return undefined;
    return state.taskSessions.items[sessionId]?.worktree_path;
  });

  const openEditor = useOpenSessionInEditor(sessionId);
  const openFolder = useOpenSessionFolder(sessionId);
  const { editors } = useEditors();
  const defaultEditorId = useAppStore((state) => state.userSettings.defaultEditorId);

  const enabledEditors = useMemo(
    () =>
      editors.filter((editor: EditorOption) => {
        if (!editor.enabled) return false;
        if (editor.kind === 'built_in') return editor.installed;
        return true;
      }),
    [editors]
  );

  const absolutePath = useMemo(() => {
    if (!worktreePath) return filePath;
    // Avoid double slashes
    const base = worktreePath.endsWith('/') ? worktreePath : `${worktreePath}/`;
    const rel = filePath.startsWith('/') ? filePath.slice(1) : filePath;
    return `${base}${rel}`;
  }, [worktreePath, filePath]);

  const handleCopyPath = useCallback(() => {
    navigator.clipboard.writeText(absolutePath);
    onCopied?.();
  }, [absolutePath, onCopied]);

  const handleOpenInEditor = useCallback(
    (editorId: string) => {
      void openEditor.open({ editorId, filePath });
    },
    [openEditor, filePath]
  );

  const handleOpenFolder = useCallback(() => {
    void openFolder.open();
  }, [openFolder]);

  const btnClass = size === 'xs'
    ? 'h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100'
    : 'h-8 w-8 p-0 cursor-pointer text-muted-foreground hover:text-foreground';

  const iconClass = size === 'xs' ? 'h-3.5 w-3.5' : 'h-4 w-4';

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" className={btnClass}>
              <IconExternalLink className={iconClass} />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>Open with...</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end" className="w-44">
        {enabledEditors.map((editor: EditorOption) => (
          <DropdownMenuItem
            key={editor.id}
            className="cursor-pointer text-xs"
            onClick={() => handleOpenInEditor(editor.id)}
          >
            {editor.name}
            {editor.id === defaultEditorId && (
              <span className="ml-auto text-[10px] text-muted-foreground">default</span>
            )}
          </DropdownMenuItem>
        ))}
        {enabledEditors.length === 0 && (
          <DropdownMenuItem disabled className="text-xs">
            No editors configured
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem className="cursor-pointer text-xs" onClick={handleCopyPath}>
          <IconCopy className="h-3.5 w-3.5 mr-1.5" />
          Copy path
        </DropdownMenuItem>
        <DropdownMenuItem className="cursor-pointer text-xs" onClick={handleOpenFolder}>
          <IconFolderShare className="h-3.5 w-3.5 mr-1.5" />
          Open folder
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
