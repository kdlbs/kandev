'use client';

import { memo } from 'react';
import type { IDockviewPanelProps } from 'dockview-react';
import { PanelRoot, PanelBody } from './panel-primitives';
import { FileEditorContent } from './file-editor-content';
import { FileImageViewer } from './file-image-viewer';
import { FileBinaryViewer } from './file-binary-viewer';
import { useAppStore } from '@/components/state-provider';
import { useDockviewStore } from '@/lib/state/dockview-store';
import { useFileEditors } from '@/hooks/use-file-editors';
import { getFileCategory } from '@/lib/utils/file-types';

export const FileEditorPanel = memo(function FileEditorPanel(
  props: IDockviewPanelProps<{ path: string }>
) {
  const path = props.params.path;
  const fileState = useDockviewStore((s) => s.openFiles.get(path));
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );
  const { savingFiles, handleFileChange, saveFile, deleteFile } = useFileEditors();

  if (!fileState) {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false} className="flex items-center justify-center text-muted-foreground text-sm">
          Loading file...
        </PanelBody>
      </PanelRoot>
    );
  }

  const extCategory = getFileCategory(path);
  const category = fileState.isBinary
    ? extCategory === 'image'
      ? 'image'
      : 'binary'
    : 'text';

  if (category === 'image') {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <FileImageViewer
            path={path}
            content={fileState.content}
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
          content={fileState.content}
          originalContent={fileState.originalContent}
          isDirty={fileState.isDirty}
          isSaving={savingFiles.has(path)}
          sessionId={activeSessionId || undefined}
          worktreePath={activeSession?.worktree_path ?? undefined}
          enableComments={!!activeSessionId}
          onChange={(newContent) => handleFileChange(path, newContent)}
          onSave={() => saveFile(path)}
          onDelete={() => deleteFile(path)}
        />
      </PanelBody>
    </PanelRoot>
  );
});
