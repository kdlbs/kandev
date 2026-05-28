"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { FileEditorContent } from "./file-editor-content";
import { FileImageViewer } from "./file-image-viewer";
import { FileBinaryViewer } from "./file-binary-viewer";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { getFileCategory } from "@/lib/utils/file-types";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { calculateHash } from "@/lib/utils/file-diff";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { syncOpenFileFromWorkspace } from "@/hooks/file-editors-sync";

type FileCategory = "image" | "binary" | "text";

function isMarkdownFile(path: string): boolean {
  const ext = path.split(".").pop()?.toLowerCase();
  return ext === "md" || ext === "mdx";
}

function resolveFileCategory(isBinary: boolean, path: string): FileCategory {
  if (!isBinary) return "text";
  return getFileCategory(path) === "image" ? "image" : "binary";
}

function ImagePanel({ path, worktreePath }: { path: string; worktreePath: string | undefined }) {
  const [imageContent, setImageContent] = useState<string | null>(null);
  const content =
    imageContent ??
    (() => {
      const c = useDockviewStore.getState().openFiles.get(path)?.content ?? "";
      queueMicrotask(() => setImageContent(c));
      return c;
    })();
  return (
    <PanelRoot>
      <PanelBody padding={false} scroll={false}>
        <FileImageViewer path={path} content={content} worktreePath={worktreePath} />
      </PanelBody>
    </PanelRoot>
  );
}

function useFileLoader(
  hasFile: boolean,
  activeSessionId: string | null,
  path: string,
  setFileState: (path: string, state: FileEditorState) => void,
) {
  const loadingRef = useRef(false);
  useEffect(() => {
    if (hasFile || loadingRef.current || !activeSessionId) return;
    loadingRef.current = true;
    const client = getWebSocketClient();
    if (!client) {
      loadingRef.current = false;
      return;
    }
    requestFileContent(client, activeSessionId, path)
      .then(async (response) => {
        const hash = await calculateHash(response.content);
        const name = path.split("/").pop() || path;
        const state: FileEditorState = {
          path,
          name,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
        };
        setFileState(path, state);
      })
      .catch(() => {
        /* stays on loading state */
      })
      .finally(() => {
        loadingRef.current = false;
      });
  }, [hasFile, activeSessionId, path, setFileState]);
}

/**
 * Force a workspace sync whenever the panel becomes the active dockview tab.
 *
 * Background: `useOpenFileWorkspaceSync` (mounted at the parent useFileEditors
 * level) refetches file content when gitStatus signatures change. That signal
 * arrives via the backend's workspace_tracker poll loop, which can be in
 * `PollModeSlow` (30s interval) until the gateway's focus signal upgrades it
 * to `PollModeFast` — there are two documented races in
 * `manager_subscription.go:FlushSessionMode` where the focus signal can miss
 * the mode upgrade for a brief window. When the missed window lines up with a
 * file edit, the editor shows stale content until the next slow-poll cycle.
 *
 * Tab activation is a deterministic, user-driven signal that the editor's
 * content is about to be looked at. Forcing a sync on activation closes the
 * WS-event-miss gap without depending on git polling cadence.
 *
 * Safe by construction: syncOpenFileFromWorkspace is dirty-buffer aware —
 * clean buffers get their content replaced, dirty buffers surface a Reload
 * affordance via `hasRemoteUpdate` rather than clobbering edits.
 */
function useResyncOnTabActivate(
  panelId: string,
  hasFile: boolean,
  activeSessionId: string | null,
  path: string,
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void,
) {
  useEffect(() => {
    if (!hasFile || !activeSessionId) return;
    const entry = panelPortalManager.get(panelId);
    if (!entry?.api) return;
    const disposable = entry.api.onDidActiveChange((event) => {
      if (!event.isActive) return;
      const client = getWebSocketClient();
      if (!client) return;
      void syncOpenFileFromWorkspace({ client, sessionId: activeSessionId, path, updateFileState });
    });
    return () => disposable.dispose();
  }, [panelId, hasFile, activeSessionId, path, updateFileState]);
}

type FileEditorPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

export const FileEditorPanel = memo(function FileEditorPanel({
  panelId,
  params,
}: FileEditorPanelProps) {
  const path = params.path as string;

  const hasFile = useDockviewStore((s) => s.openFiles.has(path));
  const content = useDockviewStore((s) => s.openFiles.get(path)?.content ?? "");
  const isDirty = useDockviewStore((s) => s.openFiles.get(path)?.isDirty ?? false);
  const hasRemoteUpdate = useDockviewStore((s) => s.openFiles.get(path)?.hasRemoteUpdate ?? false);
  const isBinary = useDockviewStore((s) => s.openFiles.get(path)?.isBinary ?? false);
  const originalContent = useDockviewStore((s) => s.openFiles.get(path)?.originalContent ?? "");
  const markdownPreview = useDockviewStore((s) => s.openFiles.get(path)?.markdownPreview ?? false);
  const setFileState = useDockviewStore((s) => s.setFileState);
  const updateFileState = useDockviewStore((s) => s.updateFileState);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );
  const gitStatus = useSessionGitStatus(activeSessionId);
  const vcsDiff = gitStatus?.files?.[path]?.diff;
  const { savingFiles, handleFileChange, saveFile, deleteFile, applyRemoteUpdate } =
    useFileEditors();
  useFileLoader(hasFile, activeSessionId, path, setFileState);
  useResyncOnTabActivate(panelId, hasFile, activeSessionId, path, updateFileState);

  const onChange = useCallback(
    (newContent: string) => handleFileChange(path, newContent),
    [handleFileChange, path],
  );
  const onSave = useCallback(() => saveFile(path), [saveFile, path]);
  const onReloadFromAgent = useCallback(() => applyRemoteUpdate(path), [applyRemoteUpdate, path]);
  const onDelete = useCallback(() => deleteFile(path), [deleteFile, path]);
  const onToggleMarkdownPreview = useCallback(
    () => updateFileState(path, { markdownPreview: !markdownPreview }),
    [updateFileState, path, markdownPreview],
  );

  if (!hasFile) {
    return (
      <PanelRoot>
        <PanelBody
          padding={false}
          scroll={false}
          className="flex items-center justify-center text-muted-foreground text-sm"
        >
          Loading file...
        </PanelBody>
      </PanelRoot>
    );
  }

  const worktreePath = activeSession?.worktree_path ?? undefined;
  const category = resolveFileCategory(isBinary, path);

  if (category === "image") return <ImagePanel path={path} worktreePath={worktreePath} />;

  if (category === "binary") {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <FileBinaryViewer path={path} worktreePath={worktreePath} />
        </PanelBody>
      </PanelRoot>
    );
  }

  const isMarkdown = isMarkdownFile(path);

  return (
    <PanelRoot>
      <PanelBody padding={false} scroll={false}>
        <FileEditorContent
          path={path}
          content={content}
          originalContent={originalContent}
          isDirty={isDirty}
          hasRemoteUpdate={hasRemoteUpdate}
          vcsDiff={vcsDiff}
          isSaving={savingFiles.has(path)}
          sessionId={activeSessionId || undefined}
          worktreePath={worktreePath}
          enableComments={!!activeSessionId}
          markdownPreview={isMarkdown ? markdownPreview : false}
          onToggleMarkdownPreview={isMarkdown ? onToggleMarkdownPreview : undefined}
          onChange={onChange}
          onSave={onSave}
          onReloadFromAgent={onReloadFromAgent}
          onDelete={onDelete}
        />
      </PanelBody>
    </PanelRoot>
  );
});
