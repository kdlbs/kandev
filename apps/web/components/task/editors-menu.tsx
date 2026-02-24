"use client";

import { useMemo } from "react";
import { IconChevronDown, IconCode, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useEditors } from "@/hooks/domains/settings/use-editors";
import { useOpenSessionInEditor } from "@/hooks/use-open-session-in-editor";
import { useAppStore } from "@/components/state-provider";
import type { EditorOption } from "@/lib/types/http";

type EditorsMenuProps = {
  activeSessionId: string | null;
};

export function EditorsMenu({ activeSessionId }: EditorsMenuProps) {
  const openEditor = useOpenSessionInEditor(activeSessionId ?? null);
  const { editors } = useEditors();
  const defaultEditorId = useAppStore((state) => state.userSettings.defaultEditorId);

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

  return (
    <div className="inline-flex rounded-md border border-border overflow-hidden">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="rounded-none border-0 cursor-pointer px-2"
            onClick={() => {
              if (!resolvedEditorId) return;
              void openEditor.open({ editorId: resolvedEditorId });
            }}
            disabled={!activeSessionId || openEditor.isLoading || enabledEditors.length === 0}
          >
            {openEditor.isLoading ? (
              <IconLoader2 className="h-4 w-4 animate-spin" />
            ) : (
              <IconCode className="h-4 w-4" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {activeSessionId ? "Open editor" : "Select a session to open its worktree"}
        </TooltipContent>
      </Tooltip>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="rounded-none border-0 border-l px-2 cursor-pointer"
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
              <DropdownMenuItem
                key={editor.id}
                className="cursor-pointer"
                onClick={() => {
                  void openEditor.open({ editorId: editor.id });
                }}
              >
                {editor.name}
              </DropdownMenuItem>
            ))
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
