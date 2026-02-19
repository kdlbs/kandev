'use client';

import { memo, useCallback, useEffect, useRef, useState } from 'react';
import type { IDockviewPanelProps } from 'dockview-react';
import { PanelRoot, PanelBody } from './panel-primitives';
import { FileEditorContent } from './file-editor-content';
import { FileImageViewer } from './file-image-viewer';
import { FileBinaryViewer } from './file-binary-viewer';
import { useAppStore } from '@/components/state-provider';
import { useDockviewStore, type FileEditorState } from '@/lib/state/dockview-store';
import { useFileEditors } from '@/hooks/use-file-editors';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { getFileCategory } from '@/lib/utils/file-types';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileContent } from '@/lib/ws/workspace-files';
import { calculateHash } from '@/lib/utils/file-diff';

type FileCategory = 'image' | 'binary' | 'text';

function resolveFileCategory(isBinary: boolean, path: string): FileCategory {
  if (!isBinary) return 'text';
  return getFileCategory(path) === 'image' ? 'image' : 'binary';
}

function ImagePanel({ path, worktreePath }: { path: string; worktreePath: string | undefined }) {
  const [imageContent, setImageContent] = useState<string | null>(null);
  const content = imageContent ?? (() => {
    const c = useDockviewStore.getState().openFiles.get(path)?.content ?? '';
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

export const FileEditorPanel = memo(function FileEditorPanel(
  props: IDockviewPanelProps<{ path: string }>
) {
  const path = props.params.path;

  const hasFile = useDockviewStore((s) => s.openFiles.has(path));
  const content = useDockviewStore((s) => s.openFiles.get(path)?.content ?? '');
  const isDirty = useDockviewStore((s) => s.openFiles.get(path)?.isDirty ?? false);
  const hasRemoteUpdate = useDockviewStore((s) => s.openFiles.get(path)?.hasRemoteUpdate ?? false);
  const isBinary = useDockviewStore((s) => s.openFiles.get(path)?.isBinary ?? false);
  const originalContent = useDockviewStore((s) => s.openFiles.get(path)?.originalContent ?? '');
  const setFileState = useDockviewStore((s) => s.setFileState);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const gitStatus = useSessionGitStatus(activeSessionId);
  const vcsDiff = gitStatus?.files?.[path]?.diff;
  const { savingFiles, handleFileChange, saveFile, deleteFile, applyRemoteUpdate } = useFileEditors();

  const loadingRef = useRef(false);
  useEffect(() => {
    if (hasFile || loadingRef.current || !activeSessionId) return;
    loadingRef.current = true;
    const client = getWebSocketClient();
    if (!client) { loadingRef.current = false; return; }
    requestFileContent(client, activeSessionId, path)
      .then(async (response) => {
        const hash = await calculateHash(response.content);
        const name = path.split('/').pop() || path;
        const state: FileEditorState = {
          path, name, content: response.content, originalContent: response.content,
          originalHash: hash, isDirty: false, isBinary: response.is_binary,
        };
        setFileState(path, state);
      })
      .catch(() => { /* stays on loading state */ })
      .finally(() => { loadingRef.current = false; });
  }, [hasFile, activeSessionId, path, setFileState]);

  const onChange = useCallback((newContent: string) => handleFileChange(path, newContent), [handleFileChange, path]);
  const onSave = useCallback(() => saveFile(path), [saveFile, path]);
  const onReloadFromAgent = useCallback(() => applyRemoteUpdate(path), [applyRemoteUpdate, path]);
  const onDelete = useCallback(() => deleteFile(path), [deleteFile, path]);

  if (!hasFile) {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false} className="flex items-center justify-center text-muted-foreground text-sm">
          Loading file...
        </PanelBody>
      </PanelRoot>
    );
  }

  const worktreePath = activeSession?.worktree_path ?? undefined;
  const category = resolveFileCategory(isBinary, path);

  if (category === 'image') return <ImagePanel path={path} worktreePath={worktreePath} />;

  if (category === 'binary') {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <FileBinaryViewer path={path} worktreePath={worktreePath} />
        </PanelBody>
      </PanelRoot>
    );
  }

  return (
    <PanelRoot>
      <PanelBody padding={false} scroll={false}>
        <FileEditorContent
          path={path} content={content} originalContent={originalContent} isDirty={isDirty}
          hasRemoteUpdate={hasRemoteUpdate} vcsDiff={vcsDiff}
          isSaving={savingFiles.has(path)} sessionId={activeSessionId || undefined}
          worktreePath={worktreePath} enableComments={!!activeSessionId}
          onChange={onChange} onSave={onSave} onReloadFromAgent={onReloadFromAgent} onDelete={onDelete}
        />
      </PanelBody>
    </PanelRoot>
  );
});
