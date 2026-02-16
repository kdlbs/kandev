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
import { getFileCategory } from '@/lib/utils/file-types';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileContent } from '@/lib/ws/workspace-files';
import { calculateHash } from '@/lib/utils/file-diff';

export const FileEditorPanel = memo(function FileEditorPanel(
  props: IDockviewPanelProps<{ path: string }>
) {
  const path = props.params.path;

  // Subscribe to individual metadata fields — NOT the full fileState object.
  // This prevents re-renders on every keystroke (content changes don't trigger re-render).
  const hasFile = useDockviewStore((s) => s.openFiles.has(path));
  const isDirty = useDockviewStore((s) => s.openFiles.get(path)?.isDirty ?? false);
  const isBinary = useDockviewStore((s) => s.openFiles.get(path)?.isBinary ?? false);
  const originalContent = useDockviewStore((s) => s.openFiles.get(path)?.originalContent ?? '');
  const setFileState = useDockviewStore((s) => s.setFileState);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const { savingFiles, handleFileChange, saveFile, deleteFile } = useFileEditors();

  // Self-load file content when state is missing (e.g. after hot reload clears the store)
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
      .catch(() => { /* will stay on loading state */ })
      .finally(() => { loadingRef.current = false; });
  }, [hasFile, activeSessionId, path, setFileState]);

  // Stable callbacks — avoid creating new arrow functions on every render
  const onChange = useCallback(
    (newContent: string) => handleFileChange(path, newContent),
    [handleFileChange, path]
  );
  const onSave = useCallback(() => saveFile(path), [saveFile, path]);
  const onDelete = useCallback(() => deleteFile(path), [deleteFile, path]);

  // Read image content imperatively (only needed for non-editable image display)
  const [imageContent, setImageContent] = useState<string | null>(null);

  if (!hasFile) {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false} className="flex items-center justify-center text-muted-foreground text-sm">
          Loading file...
        </PanelBody>
      </PanelRoot>
    );
  }

  const extCategory = getFileCategory(path);
  const category = isBinary
    ? extCategory === 'image'
      ? 'image'
      : 'binary'
    : 'text';

  if (category === 'image') {
    // Read content imperatively for image display (not edited, so no need for reactive subscription)
    const content = imageContent ?? (() => {
      const c = useDockviewStore.getState().openFiles.get(path)?.content ?? '';
      // Schedule state update outside of render to avoid setState-during-render
      queueMicrotask(() => setImageContent(c));
      return c;
    })();
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <FileImageViewer
            path={path}
            content={content}
            worktreePath={activeSession?.worktree_path ?? undefined}
          />
        </PanelBody>
      </PanelRoot>
    );
  }

  if (category === 'binary') {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <FileBinaryViewer
            path={path}
            worktreePath={activeSession?.worktree_path ?? undefined}
          />
        </PanelBody>
      </PanelRoot>
    );
  }

  return (
    <PanelRoot>
      <PanelBody padding={false} scroll={false}>
        <FileEditorContent
          path={path}
          originalContent={originalContent}
          isDirty={isDirty}
          isSaving={savingFiles.has(path)}
          sessionId={activeSessionId || undefined}
          worktreePath={activeSession?.worktree_path ?? undefined}
          enableComments={!!activeSessionId}
          onChange={onChange}
          onSave={onSave}
          onDelete={onDelete}
        />
      </PanelBody>
    </PanelRoot>
  );
});
