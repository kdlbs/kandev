"use client";

import { memo, useCallback, useEffect, useRef } from "react";
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
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";

type FileCategory = "image" | "binary" | "text";

function isMarkdownFile(path: string): boolean {
  const ext = path.split(".").pop()?.toLowerCase();
  return ext === "md" || ext === "mdx";
}

function resolveFileCategory(isBinary: boolean, path: string): FileCategory {
  if (!isBinary) return "text";
  return getFileCategory(path) === "image" ? "image" : "binary";
}

function ImagePanel({
  fileKey,
  path,
  worktreePath,
}: {
  fileKey: string;
  path: string;
  worktreePath: string | undefined;
}) {
  const content = useDockviewStore((s) => s.openFiles.get(fileKey)?.content ?? "");
  return (
    <PanelRoot>
      <PanelBody padding={false} scroll={false}>
        <FileImageViewer path={path} content={content} worktreePath={worktreePath} />
      </PanelBody>
    </PanelRoot>
  );
}

type FileLoaderArgs = {
  hasFile: boolean;
  activeSessionId: string | null;
  fileKey: string;
  path: string;
  setFileState: (path: string, state: FileEditorState) => void;
  repo?: string;
};

function useFileLoader({
  hasFile,
  activeSessionId,
  fileKey,
  path,
  setFileState,
  repo,
}: FileLoaderArgs) {
  // Key the in-flight guard by session+path+repo. If any of them changes while
  // a fetch is running, the new effect run starts a fresh fetch (rather than
  // being silently blocked) and the stale response is dropped on arrival — so
  // content from the wrong session/repo can never land in the buffer.
  const inFlightKeyRef = useRef<string | null>(null);
  useEffect(() => {
    if (hasFile || !activeSessionId) return;
    const key = `${activeSessionId}\0${path}\0${repo ?? ""}`;
    if (inFlightKeyRef.current === key) return;
    inFlightKeyRef.current = key;
    const client = getWebSocketClient();
    if (!client) {
      inFlightKeyRef.current = null;
      return;
    }
    requestFileContent(client, activeSessionId, path, repo)
      .then(async (response) => {
        if (inFlightKeyRef.current !== key) return;
        const hash = await calculateHash(response.content);
        const name = path.split("/").pop() || path;
        const state: FileEditorState = {
          path,
          repo,
          name,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
        };
        setFileState(fileKey, state);
      })
      .catch(() => {
        /* stays on loading state */
      })
      .finally(() => {
        if (inFlightKeyRef.current === key) inFlightKeyRef.current = null;
      });
  }, [hasFile, activeSessionId, fileKey, path, setFileState, repo]);
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
type ResyncOnTabActivateArgs = {
  panelId: string;
  hasFile: boolean;
  activeSessionId: string | null;
  fileKey: string;
  path: string;
  repo: string | undefined;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
};

function useResyncOnTabActivate({
  panelId,
  hasFile,
  activeSessionId,
  fileKey,
  path,
  repo,
  updateFileState,
}: ResyncOnTabActivateArgs) {
  useEffect(() => {
    if (!hasFile || !activeSessionId) return;
    // panelPortalManager.acquire() runs in usePortalSlot's mount effect (the
    // dockview-side slot), which fires before child portals' effects, so the
    // entry is virtually always present here. There is one acceptable miss:
    // a fromJSON layout restore can swap `entry.api` for the same panelId
    // without remounting this component, which would silently leave the
    // subscription pointing at a disposed api. fromJSON is rare and
    // `useOpenFileWorkspaceSync` still covers the common polling gap, so we
    // accept that edge case rather than wiring a manager-level subscription.
    const entry = panelPortalManager.get(panelId);
    if (!entry?.api) return;
    const syncNow = () => {
      const client = getWebSocketClient();
      if (!client) return;
      void syncOpenFileFromWorkspace({
        client,
        sessionId: activeSessionId,
        fileKey,
        path,
        repo,
        updateFileState,
      });
    };
    // If the panel is already the active tab when this effect first runs,
    // onDidActiveChange won't fire (no transition), but the user is already
    // looking at the editor — sync immediately so the initial open path
    // benefits from the same WS-event-miss recovery as later activations.
    if (entry.api.isActive) syncNow();
    const disposable = entry.api.onDidActiveChange((event) => {
      if (event.isActive) syncNow();
    });
    return () => disposable.dispose();
  }, [panelId, hasFile, activeSessionId, fileKey, path, repo, updateFileState]);
}

type FileEditorPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

function useFileEditorBuffer(fileKey: string) {
  const hasFile = useDockviewStore((s) => s.openFiles.has(fileKey));
  const content = useDockviewStore((s) => s.openFiles.get(fileKey)?.content ?? "");
  const isDirty = useDockviewStore((s) => s.openFiles.get(fileKey)?.isDirty ?? false);
  const hasRemoteUpdate = useDockviewStore(
    (s) => s.openFiles.get(fileKey)?.hasRemoteUpdate ?? false,
  );
  const isBinary = useDockviewStore((s) => s.openFiles.get(fileKey)?.isBinary ?? false);
  const originalContent = useDockviewStore((s) => s.openFiles.get(fileKey)?.originalContent ?? "");
  const markdownPreview = useDockviewStore(
    (s) => s.openFiles.get(fileKey)?.markdownPreview ?? false,
  );
  return {
    hasFile,
    content,
    isDirty,
    hasRemoteUpdate,
    isBinary,
    originalContent,
    markdownPreview,
  };
}

function LoadingFilePanel() {
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

export const FileEditorPanel = memo(function FileEditorPanel({
  panelId,
  params,
}: FileEditorPanelProps) {
  const path = params.path as string;
  const repo = params.repo as string | undefined;
  const fileKey = buildRepoScopedItemId(path, repo);

  const { hasFile, content, isDirty, hasRemoteUpdate, isBinary, originalContent, markdownPreview } =
    useFileEditorBuffer(fileKey);
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
  useFileLoader({ hasFile, activeSessionId, fileKey, path, setFileState, repo });
  useResyncOnTabActivate({
    panelId,
    hasFile,
    activeSessionId,
    fileKey,
    path,
    repo,
    updateFileState,
  });

  const onChange = useCallback(
    (newContent: string) => handleFileChange(path, newContent, repo),
    [handleFileChange, path, repo],
  );
  const onSave = useCallback(() => saveFile(path, repo), [saveFile, path, repo]);
  const onReloadFromAgent = useCallback(
    () => applyRemoteUpdate(path, repo),
    [applyRemoteUpdate, path, repo],
  );
  const onDelete = useCallback(() => deleteFile(path, repo), [deleteFile, path, repo]);
  const onToggleMarkdownPreview = useCallback(
    () => updateFileState(fileKey, { markdownPreview: !markdownPreview }),
    [updateFileState, fileKey, markdownPreview],
  );

  if (!hasFile) {
    return <LoadingFilePanel />;
  }

  const worktreePath = activeSession?.worktree_path ?? undefined;
  const category = resolveFileCategory(isBinary, path);

  if (category === "image")
    return <ImagePanel fileKey={fileKey} path={path} worktreePath={worktreePath} />;

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
          isSaving={savingFiles.has(fileKey)}
          sessionId={activeSessionId || undefined}
          worktreePath={worktreePath}
          repo={repo}
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
