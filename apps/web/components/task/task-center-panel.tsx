'use client';

import { memo, useCallback, useState, useEffect, useMemo } from 'react';
import { TabsContent } from '@kandev/ui/tabs';
import { Textarea } from '@kandev/ui/textarea';
import { SessionPanel } from '@kandev/ui/pannel-session';
import { TaskChatPanel } from './task-chat-panel';
import { TaskChangesPanel } from './task-changes-panel';
import { FileViewerContent } from './file-viewer-content';
import { PassthroughTerminal } from './passthrough-terminal';
import type { OpenFileTab } from '@/lib/types/backend';
import { FILE_EXTENSION_COLORS } from '@/lib/types/backend';
import { useAppStore } from '@/components/state-provider';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';

import type { SelectedDiff } from './task-layout';

type TaskCenterPanelProps = {
  selectedDiff: SelectedDiff | null;
  openFileRequest: OpenFileTab | null;
  onDiffHandled: () => void;
  onFileOpenHandled: () => void;
  sessionId?: string | null;
};

export const TaskCenterPanel = memo(function TaskCenterPanel({
  selectedDiff: externalSelectedDiff,
  openFileRequest,
  onDiffHandled,
  onFileOpenHandled,
  sessionId = null,
}: TaskCenterPanelProps) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId] ?? null : null
  );

  // Check if session is in passthrough mode by looking at the profile snapshot
  const isPassthroughMode = useMemo(() => {
    if (!activeSession?.agent_profile_snapshot) return false;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const snapshot = activeSession.agent_profile_snapshot as any;
    return snapshot?.cli_passthrough === true;
  }, [activeSession?.agent_profile_snapshot]);
  const [leftTab, setLeftTab] = useState('chat');
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);
  const [notes, setNotes] = useState('');
  const [openFileTabs, setOpenFileTabs] = useState<OpenFileTab[]>([]);

  // Handle external diff selection
  useEffect(() => {
    if (externalSelectedDiff) {
      queueMicrotask(() => {
        setSelectedDiff(externalSelectedDiff);
        setLeftTab('changes');
        onDiffHandled();
      });
    }
  }, [externalSelectedDiff, onDiffHandled]);

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
            selectedDiff={selectedDiff}
            onClearSelected={() => setSelectedDiff(null)}
          />
        </TabsContent>

        <TabsContent value="chat" className="flex flex-col min-h-0 flex-1" style={{ minHeight: '200px' }}>
          {activeTaskId ? (
            isPassthroughMode ? (
              <div className="flex-1 min-h-0 h-full" style={{ minHeight: '150px' }}>
                <PassthroughTerminal key={activeSessionId} sessionId={sessionId} />
              </div>
            ) : (
              <TaskChatPanel sessionId={sessionId} />
            )
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
