'use client';

import { memo, useState, useCallback } from 'react';
import { PanelRoot, PanelBody } from './panel-primitives';
import { useAppStore } from '@/components/state-provider';
import { useFileOperations } from '@/hooks/use-file-operations';
import { FileBrowser } from '@/components/task/file-browser';
import type { OpenFileTab } from '@/lib/types/backend';
import { useIsTaskArchived, ArchivedPanelPlaceholder } from './task-archived-context';

type FilesPanelProps = {
  onOpenFile: (file: OpenFileTab) => void;
};

const FilesPanel = memo(function FilesPanel({ onOpenFile }: FilesPanelProps) {
  const [isSearchOpen, setIsSearchOpen] = useState(false);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const isArchived = useIsTaskArchived();
  const { createFile, deleteFile } = useFileOperations(activeSessionId ?? null);

  const handleCreateFile = useCallback(async (path: string): Promise<boolean> => {
    const ok = await createFile(path);
    if (ok) {
      const name = path.split('/').pop() || path;
      const { calculateHash } = await import('@/lib/utils/file-diff');
      const hash = await calculateHash('');
      onOpenFile({ path, name, content: '', originalContent: '', originalHash: hash, isDirty: false });
    }
    return ok;
  }, [createFile, onOpenFile]);

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <PanelBody padding={false}>
        {activeSessionId ? (
          <FileBrowser
            sessionId={activeSessionId}
            onOpenFile={onOpenFile}
            onCreateFile={handleCreateFile}
            onDeleteFile={deleteFile}
            isSearchOpen={isSearchOpen}
            onCloseSearch={() => setIsSearchOpen(false)}
          />
        ) : (
          <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
            No task selected
          </div>
        )}
      </PanelBody>
    </PanelRoot>
  );
});

export { FilesPanel };
