'use client';

import { memo, useMemo, useState } from 'react';
import {
  IconArrowBackUp,
  IconExternalLink,
} from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@kandev/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { LineStat } from '@/components/diff-stat';
import { cn } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import type { FileInfo } from '@/lib/state/store';
import { FileBrowser } from '@/components/task/file-browser';

type OpenFileTab = {
  path: string;
  name: string;
  content: string;
};

type TaskFilesPanelProps = {
  taskId: string | null;
  onSelectDiffPath: (path: string) => void;
  onOpenFile: (file: OpenFileTab) => void;
};

const badgeClass = (status: string) =>
  cn(
    'text-[10px] font-semibold',
    (status === 'M' || status === 'modified') && 'bg-yellow-500/15 text-yellow-700',
    (status === 'A' || status === 'added') && 'bg-emerald-500/15 text-emerald-700',
    (status === 'D' || status === 'deleted') && 'bg-rose-500/15 text-rose-700',
    status === 'untracked' && 'bg-blue-500/15 text-blue-700',
    status === 'renamed' && 'bg-purple-500/15 text-purple-700'
  );

const statusLabel = (status: FileInfo['status']): string => {
  switch (status) {
    case 'modified': return 'M';
    case 'added': return 'A';
    case 'deleted': return 'D';
    case 'untracked': return 'U';
    case 'renamed': return 'R';
    default: return '?';
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

const TaskFilesPanel = memo(function TaskFilesPanel({ taskId, onSelectDiffPath, onOpenFile }: TaskFilesPanelProps) {
  const [topTab, setTopTab] = useState<'diff' | 'files'>('diff');
  const gitStatus = useAppStore((state) => state.gitStatus);

  // Convert git status files to array for display
  const changedFiles = useMemo(() => {
    if (!gitStatus.files || Object.keys(gitStatus.files).length === 0) {
      return [];
    }
    return Object.values(gitStatus.files).map((file) => ({
      path: file.path,
      status: file.status,
      plus: file.additions,
      minus: file.deletions,
      oldPath: file.old_path,
    }));
  }, [gitStatus.files]);

  return (
    <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0">
      <Tabs value={topTab} onValueChange={(value) => setTopTab(value as 'diff' | 'files')} className="flex-1 min-h-0">
        <TabsList>
          <TabsTrigger value="diff" className="cursor-pointer">
            Diff files {changedFiles.length > 0 && `(${changedFiles.length})`}
          </TabsTrigger>
          <TabsTrigger value="files" className="cursor-pointer">
            All files
          </TabsTrigger>
        </TabsList>
        <TabsContent value="diff" className="mt-3 flex-1 min-h-0">
          <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
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
                                }}
                              >
                                <IconExternalLink className="h-3.5 w-3.5" />
                              </button>
                            </TooltipTrigger>
                            <TooltipContent>Open in editor</TooltipContent>
                          </Tooltip>
                        </div>
                        <LineStat added={file.plus} removed={file.minus} />
                        <Badge className={badgeClass(file.status)}>{statusLabel(file.status)}</Badge>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </TabsContent>
        <TabsContent value="files" className="mt-3 flex-1 min-h-0">
          <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background h-full">
            {taskId ? (
              <FileBrowser taskId={taskId} onOpenFile={onOpenFile} />
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                No task selected
              </div>
            )}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
});

export { TaskFilesPanel };
