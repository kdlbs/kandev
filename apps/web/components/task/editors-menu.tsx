"use client";

import { useMemo } from "react";
import { IconChevronDown, IconCode, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useEditors } from "@/hooks/domains/settings/use-editors";
import { useOpenSessionInEditor } from "@/hooks/use-open-session-in-editor";
import { useSessionWorktrees } from "@/hooks/domains/session/use-session-worktrees";
import { useAppStore } from "@/components/state-provider";
import type { EditorOption } from "@/lib/types/http";
import {
  buildWorktreeOptions,
  type WorktreeOption,
} from "@/components/task/editor-worktree-options";

const menuItemClass = "cursor-pointer";

type EditorsMenuProps = {
  activeSessionId: string | null;
};

function useWorktreeOptions(sessionId: string | null): WorktreeOption[] {
  const worktrees = useSessionWorktrees(sessionId);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  return useMemo(
    () => buildWorktreeOptions(worktrees, Object.values(repositoriesByWorkspace).flat()),
    [worktrees, repositoriesByWorkspace],
  );
}

function WorktreeItems({
  options,
  onSelect,
}: {
  options: WorktreeOption[];
  onSelect: (worktreeId: string) => void;
}) {
  return (
    <>
      {options.map((option) => (
        <DropdownMenuItem
          key={option.worktreeId}
          className={menuItemClass}
          data-testid="editors-menu-worktree-item"
          onClick={() => onSelect(option.worktreeId)}
        >
          <span className="truncate">{option.label}</span>
          {option.branch && (
            <span className="ml-auto pl-3 text-xs text-muted-foreground truncate max-w-40">
              {option.branch}
            </span>
          )}
        </DropdownMenuItem>
      ))}
    </>
  );
}

function OpenEditorButton({
  disabled,
  isLoading,
  tooltip,
  worktreeOptions,
  onOpen,
}: {
  disabled: boolean;
  isLoading: boolean;
  tooltip: string;
  worktreeOptions: WorktreeOption[];
  onOpen: (worktreeId?: string) => void;
}) {
  const icon = isLoading ? (
    <IconLoader2 className="h-4 w-4 animate-spin" />
  ) : (
    <IconCode className="h-4 w-4" />
  );
  const buttonClass = "rounded-none border-0 cursor-pointer px-2 focus-visible:ring-inset";

  // Multi-worktree sessions get a picker instead of opening the first worktree.
  if (worktreeOptions.length > 1) {
    return (
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className={buttonClass}
                data-testid="editors-menu-open"
                disabled={disabled}
              >
                {icon}
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>{tooltip}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end" className="min-w-56 w-auto">
          <WorktreeItems options={worktreeOptions} onSelect={(worktreeId) => onOpen(worktreeId)} />
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className={buttonClass}
          data-testid="editors-menu-open"
          onClick={() => onOpen()}
          disabled={disabled}
        >
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function EditorMenuEntry({
  editor,
  worktreeOptions,
  onOpen,
}: {
  editor: EditorOption;
  worktreeOptions: WorktreeOption[];
  onOpen: (editorId: string, worktreeId?: string) => void;
}) {
  if (worktreeOptions.length > 1) {
    return (
      <DropdownMenuSub>
        <DropdownMenuSubTrigger className={menuItemClass}>{editor.name}</DropdownMenuSubTrigger>
        <DropdownMenuSubContent className="min-w-56 w-auto">
          <WorktreeItems
            options={worktreeOptions}
            onSelect={(worktreeId) => onOpen(editor.id, worktreeId)}
          />
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    );
  }
  return (
    <DropdownMenuItem className={menuItemClass} onClick={() => onOpen(editor.id)}>
      {editor.name}
    </DropdownMenuItem>
  );
}

export function EditorsMenu({ activeSessionId }: EditorsMenuProps) {
  const openEditor = useOpenSessionInEditor(activeSessionId ?? null);
  const { editors } = useEditors();
  const defaultEditorId = useAppStore((state) => state.userSettings.defaultEditorId);
  const worktreeOptions = useWorktreeOptions(activeSessionId ?? null);

  const enabledEditors = useMemo(
    () =>
      editors.filter((editor: EditorOption) => {
        if (!editor.enabled) return false;
        if (editor.kind === "built_in") return editor.installed;
        return true;
      }),
    [editors],
  );

  const resolvedEditorId = useMemo(() => {
    if (
      defaultEditorId &&
      enabledEditors.some((editor: EditorOption) => editor.id === defaultEditorId)
    ) {
      return defaultEditorId;
    }
    return enabledEditors[0]?.id ?? "";
  }, [defaultEditorId, enabledEditors]);

  const openWith = (editorId: string, worktreeId?: string) => {
    if (!editorId) return;
    void openEditor.open({ editorId, worktreeId });
  };

  return (
    <div className="inline-flex h-7 rounded-md border border-border overflow-hidden">
      <OpenEditorButton
        disabled={!activeSessionId || openEditor.isLoading || enabledEditors.length === 0}
        isLoading={openEditor.isLoading}
        tooltip={activeSessionId ? "Open editor" : "Select a session to open its worktree"}
        worktreeOptions={worktreeOptions}
        onOpen={(worktreeId) => openWith(resolvedEditorId, worktreeId)}
      />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="rounded-none border-0 border-l px-2 cursor-pointer focus-visible:ring-inset"
            data-testid="editors-menu-list"
            disabled={!activeSessionId || enabledEditors.length === 0}
          >
            <IconChevronDown className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {enabledEditors.length === 0 ? (
            <DropdownMenuItem disabled>No editors available</DropdownMenuItem>
          ) : (
            enabledEditors.map((editor: EditorOption) => (
              <EditorMenuEntry
                key={editor.id}
                editor={editor}
                worktreeOptions={worktreeOptions}
                onOpen={openWith}
              />
            ))
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
