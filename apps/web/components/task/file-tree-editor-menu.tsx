"use client";

import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef } from "react";
import { IconCode } from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import { useEditors } from "@/hooks/domains/settings/use-editors";
import { useOpenSessionInEditor } from "@/hooks/use-open-session-in-editor";
import { useSessionWorktrees } from "@/hooks/domains/session/use-session-worktrees";
import { useAppStore } from "@/components/state-provider";
import type { EditorOption } from "@/lib/types/http";
import type { FileTreeNode } from "@/lib/types/backend";
import {
  getAvailableTaskTopbarEditors,
  resolveTaskTopbarEditorId,
} from "@/components/task/editors-menu-availability";
import { resolveFileTreeEditorTarget, type FileTreeEditorTarget } from "./file-tree-editor-target";

export type FileTreeEditorActions = {
  editors: EditorOption[];
  defaultEditorId: string;
  /** `null` when the node has no worktree the editors API can resolve against. */
  resolveTarget: (node: FileTreeNode) => FileTreeEditorTarget | null;
  openInEditor: (node: FileTreeNode, editorId: string) => void;
};

export const FileTreeEditorContext = createContext<FileTreeEditorActions | null>(null);

export function useFileTreeEditorActions(): FileTreeEditorActions | null {
  return useContext(FileTreeEditorContext);
}

/**
 * Supplies the Files panel context menus with the session's available editors
 * and an opener that resolves a tree node path to the editors API's
 * `{worktreeId, filePath}` pair. Provided once per browser rather than per row
 * so every tree node shares a single set of editor subscriptions.
 */
export function FileTreeEditorProvider({
  sessionId,
  treeRootName,
  children,
}: {
  sessionId: string;
  /** Name of the tree's root directory, used to tell which root it is served from. */
  treeRootName?: string;
  children: React.ReactNode;
}) {
  const { editors } = useEditors();
  const defaultEditorId = useAppStore((state) => state.userSettings.defaultEditorId);
  const worktrees = useSessionWorktrees(sessionId);
  // Published by the task page from the session status; the Files panel is
  // rendered inside the dock layout and cannot reach that state directly.
  const embeddedVscodeSupported = useAppStore(
    (state) => state.embeddedVscodeSupport.bySessionId[sessionId] ?? false,
  );
  const openEditor = useOpenSessionInEditor(sessionId);

  // `open` is a fresh closure on every render; keep the context value stable by
  // calling through a ref instead of depending on it directly.
  const openRef = useRef(openEditor.open);
  useEffect(() => {
    openRef.current = openEditor.open;
  });

  const availableEditors = useMemo(
    () => getAvailableTaskTopbarEditors(editors, embeddedVscodeSupported),
    [editors, embeddedVscodeSupported],
  );

  const resolveTarget = useCallback(
    (node: FileTreeNode) => resolveFileTreeEditorTarget(node.path, worktrees, treeRootName),
    [worktrees, treeRootName],
  );

  const openInEditor = useCallback(
    (node: FileTreeNode, editorId: string) => {
      const target = resolveTarget(node);
      if (!editorId || !target) return;
      void openRef.current({
        editorId,
        filePath: target.filePath,
        worktreeId: target.worktreeId,
        isDirectory: node.is_dir,
      });
    },
    [resolveTarget],
  );

  const value = useMemo<FileTreeEditorActions>(
    () => ({
      editors: availableEditors,
      defaultEditorId: resolveTaskTopbarEditorId(defaultEditorId, availableEditors),
      resolveTarget,
      openInEditor,
    }),
    [availableEditors, defaultEditorId, resolveTarget, openInEditor],
  );

  return <FileTreeEditorContext.Provider value={value}>{children}</FileTreeEditorContext.Provider>;
}

/**
 * "Open in <editor>" entries for a single tree node, plus a submenu with the
 * remaining editors when more than one is available.
 */
export function OpenInEditorMenuItems({ node }: { node: FileTreeNode }) {
  const actions = useFileTreeEditorActions();
  if (!actions) return null;
  const { editors, defaultEditorId, openInEditor } = actions;
  const primary = editors.find((editor) => editor.id === defaultEditorId);
  if (!primary) return null;
  const others = editors.filter((editor) => editor.id !== primary.id);

  return (
    <>
      <ContextMenuItem
        data-testid="file-tree-open-in-editor"
        onSelect={() => openInEditor(node, primary.id)}
      >
        <IconCode className="h-3.5 w-3.5" />
        Open in {primary.name}
      </ContextMenuItem>
      {others.length > 0 && (
        <ContextMenuSub>
          <ContextMenuSubTrigger data-testid="file-tree-open-in-other-editor">
            Open in other editor
          </ContextMenuSubTrigger>
          <ContextMenuSubContent>
            {others.map((editor) => (
              <ContextMenuItem key={editor.id} onSelect={() => openInEditor(node, editor.id)}>
                {editor.name}
              </ContextMenuItem>
            ))}
          </ContextMenuSubContent>
        </ContextMenuSub>
      )}
    </>
  );
}

export function canOpenNodeInEditor(
  actions: FileTreeEditorActions | null,
  node: FileTreeNode,
): boolean {
  if (!actions) return false;
  if (!actions.editors.some((editor) => editor.id === actions.defaultEditorId)) return false;
  return actions.resolveTarget(node) !== null;
}
