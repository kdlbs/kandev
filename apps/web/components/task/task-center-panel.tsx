'use client';

import { memo, useCallback, useState, useEffect, useMemo } from 'react';
import { TabsContent } from '@kandev/ui/tabs';
import { Textarea } from '@kandev/ui/textarea';
import { SessionPanel } from '@kandev/ui/pannel-session';
import { TaskChatPanel } from './task-chat-panel';
import { TaskChangesPanel } from './task-changes-panel';
import { FileViewerContent } from './file-viewer-content';
import type { OpenFileTab } from '@/lib/types/backend';
import { FILE_EXTENSION_COLORS } from '@/lib/types/backend';
import { useAppStore } from '@/components/state-provider';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';

type TaskCenterPanelProps = {
  selectedDiffPath: string | null;
  openFileRequest: OpenFileTab | null;
  onDiffPathHandled: () => void;
  onFileOpenHandled: () => void;
  sessionId?: string | null;
};

export const TaskCenterPanel = memo(function TaskCenterPanel({
  selectedDiffPath: externalSelectedDiffPath,
  openFileRequest,
  onDiffPathHandled,
  onFileOpenHandled,
  sessionId = null,
}: TaskCenterPanelProps) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const [leftTab, setLeftTab] = useState('chat');
  const [selectedDiffPath, setSelectedDiffPath] = useState<string | null>(null);
  const [notes, setNotes] = useState('');
  const [openFileTabs, setOpenFileTabs] = useState<OpenFileTab[]>([]);

  // Handle external diff path selection
  useEffect(() => {
    if (externalSelectedDiffPath) {
      queueMicrotask(() => {
        setSelectedDiffPath(externalSelectedDiffPath);
        setLeftTab('changes');
        onDiffPathHandled();
      });
    }
  }, [externalSelectedDiffPath, onDiffPathHandled]);

  // Handle external file open request
  useEffect(() => {
    if (openFileRequest) {
      queueMicrotask(() => {
        setOpenFileTabs((prev) => {
          // If file is already open, just switch to it
          if (prev.some((t) => t.path === openFileRequest.path)) {
            return prev;
          }
          // Add new tab (LRU eviction at max 4)
          const maxTabs = 4;
          const newTabs = prev.length >= maxTabs ? [...prev.slice(1), openFileRequest] : [...prev, openFileRequest];
          return newTabs;
        });
        setLeftTab(`file:${openFileRequest.path}`);
        onFileOpenHandled();
      });
    }
  }, [openFileRequest, onFileOpenHandled]);

  const handleCloseFileTab = useCallback((path: string) => {
    setOpenFileTabs((prev) => prev.filter((t) => t.path !== path));
    // If closing the active tab, switch to chat
    if (leftTab === `file:${path}`) {
      setLeftTab('chat');
    }
  }, [leftTab]);

  const tabs: SessionTab[] = useMemo(() => {
    const staticTabs: SessionTab[] = [
      { id: 'notes', label: 'Notes' },
      { id: 'changes', label: 'All changes' },
      { id: 'chat', label: 'Chat' },
    ];

    const fileTabs: SessionTab[] = openFileTabs.map((tab) => {
      const ext = tab.name.split('.').pop()?.toLowerCase() || '';
      const dotColor = FILE_EXTENSION_COLORS[ext] || 'bg-muted-foreground';
      return {
        id: `file:${tab.path}`,
        label: tab.name,
        icon: <span className={`h-2 w-2 rounded-full ${dotColor}`} />,
        closable: true,
        onClose: (e) => {
          e.stopPropagation();
          handleCloseFileTab(tab.path);
        },
        className: 'cursor-pointer group gap-1.5 data-[state=active]:bg-muted',
      };
    });

    return [...staticTabs, ...fileTabs];
  }, [openFileTabs, handleCloseFileTab]);

  return (
    <SessionPanel borderSide="right" margin="right">
      <SessionTabs
        tabs={tabs}
        activeTab={leftTab}
        onTabChange={setLeftTab}
        separatorAfterIndex={openFileTabs.length > 0 ? 2 : undefined}
        className="flex-1 min-h-0 flex flex-col gap-2"
      >

        <TabsContent value="notes" className="flex-1 min-h-0">
          <Textarea
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
            placeholder="Add task notes here..."
            className="min-h-0 h-full resize-none"
          />
        </TabsContent>

        <TabsContent value="changes" className="flex-1 min-h-0">
          <TaskChangesPanel
            selectedDiffPath={selectedDiffPath}
            onClearSelected={() => setSelectedDiffPath(null)}
          />
        </TabsContent>

        <TabsContent value="chat" className="flex flex-col min-h-0 flex-1">
          {activeTaskId ? (
            <TaskChatPanel sessionId={sessionId} />
          ) : (
            <div className="flex items-center justify-center h-full text-muted-foreground">
              No task selected
            </div>
          )}
        </TabsContent>

        {openFileTabs.map((tab) => (
          <TabsContent key={tab.path} value={`file:${tab.path}`} className="mt-3 flex-1 min-h-0">
            <FileViewerContent path={tab.path} content={tab.content} />
          </TabsContent>
        ))}
      </SessionTabs>
    </SessionPanel>
  );
});
