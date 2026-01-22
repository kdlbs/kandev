'use client';

import { memo, useMemo, useState } from 'react';
import {
  IconArrowBackUp,
  IconExternalLink,
  IconPlus,
  IconCircleFilled,
  IconMinus,
  IconCheck,
} from '@tabler/icons-react';

import { TabsContent } from '@kandev/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { SessionPanel, SessionPanelContent } from '@kandev/ui/pannel-session';
import { LineStat } from '@/components/diff-stat';
import { useAppStore } from '@/components/state-provider';
import { useOpenSessionInEditor } from '@/hooks/use-open-session-in-editor';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useGitOperations } from '@/hooks/use-git-operations';
import type { FileInfo } from '@/lib/state/store';
import { FileBrowser } from '@/components/task/file-browser';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';

type OpenFileTab = {
  path: string;
  name: string;
  content: string;
};

type TaskFilesPanelProps = {
  onSelectDiffPath: (path: string) => void;
  onOpenFile: (file: OpenFileTab) => void;
};



const StatusIcon = ({ status }: { status: FileInfo['status'] }) => {
  switch (status) {
    case 'added':
    case 'untracked':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-emerald-600">
          <IconPlus className="h-2 w-2 text-emerald-600" />
        </div>
      );
    case 'modified':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-yellow-600">
          <IconCircleFilled className="h-1 w-1 text-yellow-600" />
        </div>
      );
    case 'deleted':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-rose-600">
          <IconMinus className="h-2 w-2 text-rose-600" />
        </div>
      );
    case 'renamed':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-purple-600">
          <IconCircleFilled className="h-1 w-1 text-purple-600" />
        </div>
      );
    default:
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-muted-foreground">
          <IconCircleFilled className="h-1 w-1 text-muted-foreground" />
        </div>
      );
  }
};

const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf('/');
  if (lastSlash === -1) return { folder: '', file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

const TaskFilesPanel = memo(function TaskFilesPanel({ onSelectDiffPath, onOpenFile }: TaskFilesPanelProps) {
  const [topTab, setTopTab] = useState<'diff' | 'files'>('diff');
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const openEditor = useOpenSessionInEditor(activeSessionId ?? null);
  const gitOps = useGitOperations(activeSessionId ?? null);

  // Convert git status files to array for display
  const changedFiles = useMemo(() => {
    if (!gitStatus?.files || Object.keys(gitStatus.files).length === 0) {
      return [];
    }
    return (Object.values(gitStatus.files) as FileInfo[]).map((file: FileInfo) => ({
      path: file.path,
      status: file.status,
      staged: file.staged,
      plus: file.additions,
      minus: file.deletions,
      oldPath: file.old_path,
    }));
  }, [gitStatus]);

  const tabs: SessionTab[] = [
    {
      id: 'diff',
      label: `Diff files${changedFiles.length > 0 ? ` (${changedFiles.length})` : ''}`,
    },
    {
      id: 'files',
      label: 'All files',
    },
  ];

  return (
    <SessionPanel borderSide="left">
      <SessionTabs
        tabs={tabs}
        activeTab={topTab}
        onTabChange={(value) => setTopTab(value as 'diff' | 'files')}
        className="flex-1 min-h-0"
      >
        <TabsContent value="diff" className="mt-3 flex-1 min-h-0">
          <SessionPanelContent>
            {changedFiles.length === 0 ? (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                No changes detected
              </div>
            ) : (
              <ul className="space-y-2">
                {changedFiles.map((file) => {
                  const { folder, file: name } = splitPath(file.path);
                  return (
                    <li
                      key={file.path}
                      className="group flex items-center justify-between gap-3 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
                      onClick={() => onSelectDiffPath(file.path)}
                    >
                      <div className="flex items-center gap-2 min-w-0">
                        {/* Staged indicator */}
                        {file.staged ? (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <div className="flex items-center justify-center h-4 w-4 rounded bg-emerald-500/20 text-emerald-600">
                                <IconCheck className="h-3 w-3" />
                              </div>
                            </TooltipTrigger>
                            <TooltipContent>Staged for commit</TooltipContent>
                          </Tooltip>
                        ) : (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <button
                                type="button"
                                className="flex items-center justify-center h-4 w-4 rounded border border-dashed border-muted-foreground/50 text-muted-foreground hover:border-emerald-500 hover:text-emerald-500 hover:bg-emerald-500/10"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  void gitOps.stage([file.path]);
                                }}
                              >
                                <IconPlus className="h-2.5 w-2.5" />
                              </button>
                            </TooltipTrigger>
                            <TooltipContent>Stage file</TooltipContent>
                          </Tooltip>
                        )}
                        <button type="button" className="min-w-0 text-left cursor-pointer">
                          <p className="truncate text-foreground">
                            <span className="text-foreground/60">{folder}/</span>
                            <span className="font-medium text-foreground">{name}</span>
                          </p>
                          {file.oldPath && (
                            <p className="truncate text-xs text-muted-foreground">
                              Renamed from: {file.oldPath}
                            </p>
                          )}
                        </button>
                      </div>
                      <div className="flex items-center gap-2">
                        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <button
                                type="button"
                                className="text-muted-foreground hover:text-foreground"
                                onClick={(event) => {
                                  event.stopPropagation();
                                }}
                              >
                                <IconArrowBackUp className="h-3.5 w-3.5" />
                              </button>
                            </TooltipTrigger>
                            <TooltipContent>Discard changes</TooltipContent>
                          </Tooltip>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <button
                                type="button"
                                className="text-muted-foreground hover:text-foreground"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  if (!activeSessionId) return;
                                  void openEditor.open({ filePath: file.path });
                                }}
                              >
                                <IconExternalLink className="h-3.5 w-3.5" />
                              </button>
                            </TooltipTrigger>
                            <TooltipContent>Open in editor</TooltipContent>
                          </Tooltip>
                        </div>
                        <LineStat added={file.plus} removed={file.minus} />
                        <StatusIcon status={file.status} />
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </SessionPanelContent>
        </TabsContent>
        <TabsContent value="files" className="mt-3 flex-1 min-h-0">
          <SessionPanelContent>
            {activeSessionId ? (
              <FileBrowser sessionId={activeSessionId} onOpenFile={onOpenFile} />
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                No task selected
              </div>
            )}
          </SessionPanelContent>
        </TabsContent>
      </SessionTabs>
    </SessionPanel>
  );
});

export { TaskFilesPanel };
