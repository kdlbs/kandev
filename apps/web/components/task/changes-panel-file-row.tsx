'use client';

import {
  IconArrowBackUp,
  IconPlus,
  IconMinus,
  IconCheck,
  IconLoader2,
  IconPencil,
} from '@tabler/icons-react';

import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { LineStat } from '@/components/diff-stat';
import type { FileInfo } from '@/lib/state/store';
import { FileStatusIcon } from './file-status-icon';

const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf('/');
  if (lastSlash === -1) return { folder: '', file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

type ChangedFile = {
  path: string;
  status: FileInfo['status'];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
};

type FileRowProps = {
  file: ChangedFile;
  isPending: boolean;
  onOpenDiff: (path: string) => void;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
  onDiscard: (path: string) => void;
  onEditFile: (path: string) => void;
};

export function FileRow({
  file,
  isPending,
  onOpenDiff,
  onStage,
  onUnstage,
  onDiscard,
  onEditFile,
}: FileRowProps) {
  const { folder, file: name } = splitPath(file.path);

  return (
    <li
      className="group flex items-center justify-between gap-2 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
      onClick={() => onOpenDiff(file.path)}
    >
      <div className="flex items-center gap-2 min-w-0">
        <StageButton
          isPending={isPending}
          staged={file.staged}
          path={file.path}
          onStage={onStage}
          onUnstage={onUnstage}
        />
        <button type="button" className="min-w-0 text-left cursor-pointer" title={file.path}>
          <p className="flex text-foreground text-xs min-w-0">
            {folder && <span className="text-foreground/60 truncate shrink">{folder}/</span>}
            <span className="font-medium text-foreground whitespace-nowrap shrink-0">{name}</span>
          </p>
        </button>
      </div>
      <div className="flex items-center gap-2">
        <FileRowActions
          path={file.path}
          onDiscard={onDiscard}
          onEditFile={onEditFile}
        />
        <LineStat added={file.plus} removed={file.minus} />
        <FileStatusIcon status={file.status} />
      </div>
    </li>
  );
}

function StageButton({
  isPending,
  staged,
  path,
  onStage,
  onUnstage,
}: {
  isPending: boolean;
  staged: boolean;
  path: string;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
}) {
  if (isPending) {
    return (
      <div className="flex-shrink-0 flex items-center justify-center size-4">
        <IconLoader2 className="h-3 w-3 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (staged) {
    return (
      <button
        type="button"
        title="Unstage file"
        className="group/unstage flex-shrink-0 flex items-center justify-center size-4 rounded bg-emerald-500/20 text-emerald-600 hover:bg-rose-500/20 hover:text-rose-600 cursor-pointer"
        onClick={(e) => {
          e.stopPropagation();
          onUnstage(path);
        }}
      >
        <IconCheck className="h-3 w-3 group-hover/unstage:hidden" />
        <IconMinus className="h-2.5 w-2.5 hidden group-hover/unstage:block" />
      </button>
    );
  }
  return (
    <button
      type="button"
      title="Stage file"
      className="flex-shrink-0 flex items-center justify-center size-4 rounded border border-dashed border-muted-foreground/50 text-muted-foreground hover:border-emerald-500 hover:text-emerald-500 hover:bg-emerald-500/10 cursor-pointer"
      onClick={(e) => {
        e.stopPropagation();
        onStage(path);
      }}
    >
      <IconPlus className="h-2.5 w-2.5" />
    </button>
  );
}

function FileRowActions({
  path,
  onDiscard,
  onEditFile,
}: {
  path: string;
  onDiscard: (path: string) => void;
  onEditFile: (path: string) => void;
}) {
  return (
    <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onDiscard(path);
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
            className="text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onEditFile(path);
            }}
          >
            <IconPencil className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Edit</TooltipContent>
      </Tooltip>
    </div>
  );
}
