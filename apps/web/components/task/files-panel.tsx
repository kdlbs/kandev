'use client';

import { memo, useState, useCallback } from 'react';
import { PanelRoot, PanelBody } from './panel-primitives';
import { useAppStore } from '@/components/state-provider';
import { useToast } from '@/components/toast-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { deleteFile } from '@/lib/ws/workspace-files';
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
  const { toast } = useToast();

  const handleDeleteFile = useCallback(async (path: string) => {
    const client = getWebSocketClient();
    if (!client || !activeSessionId) return;

    try {
      const response = await deleteFile(client, activeSessionId, path);
      if (!response.success) {
        toast({
          title: 'Failed to delete file',
          description: response.error || 'An unknown error occurred',
          variant: 'error',
        });
      }
    } catch (error) {
      toast({
        title: 'Failed to delete file',
        description: error instanceof Error ? error.message : 'An unknown error occurred',
        variant: 'error',
      });
    }
  }, [activeSessionId, toast]);

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <PanelBody padding={false}>
        {activeSessionId ? (
          <FileBrowser
            sessionId={activeSessionId}
            onOpenFile={onOpenFile}
            onDeleteFile={handleDeleteFile}
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
