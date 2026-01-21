'use client';

import { memo, useCallback, useState, useEffect } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@kandev/ui/tabs';
import { Textarea } from '@kandev/ui/textarea';
import { IconX } from '@tabler/icons-react';
import { TaskChatPanel } from './task-chat-panel';
import { TaskChangesPanel } from './task-changes-panel';
import { FileViewerContent } from './file-viewer-content';
import type { OpenFileTab } from '@/lib/types/backend';
import { FILE_EXTENSION_COLORS } from '@/lib/types/backend';
import { useAppStore } from '@/components/state-provider';

const AGENTS = [
  { id: 'codex', label: 'Codex' },
  { id: 'claude', label: 'Claude Code' },
];

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

  return (
    <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-r-0 mr-[5px]">
      <Tabs
        value={leftTab}
        onValueChange={(value) => setLeftTab(value)}
        className="flex-1 min-h-0 flex flex-col"
      >
        <div className="flex items-center gap-1">
          <TabsList>
            <TabsTrigger value="notes" className="cursor-pointer">
              Notes
            </TabsTrigger>
            <TabsTrigger value="changes" className="cursor-pointer">
              All changes
            </TabsTrigger>
            <TabsTrigger value="chat" className="cursor-pointer">
              Chat
            </TabsTrigger>
          </TabsList>
          {openFileTabs.length > 0 && (
            <>
              <div className="h-4 w-px bg-border mx-1" />
              <TabsList className="bg-transparent">
                {openFileTabs.map((tab) => {
                  const ext = tab.name.split('.').pop()?.toLowerCase() || '';
                  const dotColor = FILE_EXTENSION_COLORS[ext] || 'bg-muted-foreground';
                  return (
                    <TabsTrigger
                      key={tab.path}
                      value={`file:${tab.path}`}
                      className="cursor-pointer relative group gap-1.5 data-[state=active]:bg-muted"
                    >
                      <span className={`h-2 w-2 rounded-full ${dotColor}`} />
                      <span className="truncate max-w-[100px]">{tab.name}</span>
                      <span
                        role="button"
                        tabIndex={0}
                        onClick={(e) => {
                          e.stopPropagation();
                          handleCloseFileTab(tab.path);
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            e.stopPropagation();
                            handleCloseFileTab(tab.path);
                          }
                        }}
                        className="ml-0.5 opacity-0 group-hover:opacity-100 transition-opacity hover:text-foreground !cursor-pointer"
                      >
                        <IconX className="h-3 w-3" />
                      </span>
                    </TabsTrigger>
                  );
                })}
              </TabsList>
            </>
          )}
        </div>

        <TabsContent value="notes" className="mt-3 flex-1 min-h-0">
          <Textarea
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
            placeholder="Add task notes here..."
            className="min-h-0 h-full resize-none"
          />
        </TabsContent>

        <TabsContent value="changes" className="mt-3 flex-1 min-h-0">
          <TaskChangesPanel
            selectedDiffPath={selectedDiffPath}
            onClearSelected={() => setSelectedDiffPath(null)}
          />
        </TabsContent>

        <TabsContent value="chat" className="mt-3 flex flex-col min-h-0 flex-1">
          {activeTaskId ? (
            <TaskChatPanel agents={AGENTS} sessionId={sessionId} />
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
      </Tabs>
    </div>
  );
});
