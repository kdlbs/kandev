'use client';

import { useState, useRef } from 'react';
import { IconCheck, IconX, IconLoader2, IconEdit, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import type { Message } from '@/lib/types/http';
import { DiffViewBlock } from './diff-view-block';

type FileMutation = {
  type?: string;
  old_content?: string;
  new_content?: string;
  diff?: string;
};

type ModifyFilePayload = {
  file_path?: string;
  mutations?: FileMutation[];
};

type NormalizedPayload = {
  kind?: string;
  modify_file?: ModifyFilePayload;
};

type ToolEditMetadata = {
  tool_call_id?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  normalized?: NormalizedPayload;
};

// Parse unified diff string into hunks array for DiffViewBlock
function parseDiffToHunks(diffString: string): string[] {
  if (!diffString) return [];
  // Split on hunk headers (@@ ... @@) but keep them
  const hunks = diffString.split(/(?=^@@)/m).filter(h => h.trim());
  return hunks;
}

export function ToolEditMessage({ comment }: { comment: Message }) {
  const metadata = comment.metadata as ToolEditMetadata | undefined;
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(metadata?.status);

  const status = metadata?.status;
  const normalized = metadata?.normalized;
  const filePath = normalized?.modify_file?.file_path;
  const diffString = normalized?.modify_file?.mutations?.[0]?.diff;
  const diff = diffString ? {
    hunks: parseDiffToHunks(diffString),
    oldFile: { fileName: filePath },
    newFile: { fileName: filePath },
  } : undefined;
  const isSuccess = status === 'complete';

  // Reset manual state when status changes (allows auto-expand behavior to resume)
  if (prevStatusRef.current !== status) {
    prevStatusRef.current = status;
    if (manualExpandState !== null) {
      setManualExpandState(null);
    }
  }

  // Derive expanded state: manual override takes precedence, otherwise auto-expand when running
  const isExpanded = manualExpandState ?? (status === 'running');

  const getStatusIcon = () => {
    switch (status) {
      case 'complete':
        return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
      case 'error':
        return <IconX className="h-3.5 w-3.5 text-red-500" />;
      case 'running':
        return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
      default:
        return null;
    }
  };

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full">
        <div className="flex-shrink-0 mt-0.5">
          <IconEdit className="h-4 w-4 text-muted-foreground" />
        </div>

        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <button
              type="button"
              onClick={() => {
                setManualExpandState(!isExpanded);
              }}
              className="inline-flex items-center gap-1.5 text-left cursor-pointer hover:opacity-70 transition-opacity"
            >
              <span className="font-mono text-xs">{comment.content}</span>
              {!isSuccess && getStatusIcon()}
              {diff && (isExpanded ? <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/50" /> : <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/50" />)}
            </button>
            {filePath && (
              <span className="text-xs text-muted-foreground/60 truncate">{filePath}</span>
            )}
          </div>

          {isExpanded && diff && (
            <div className="mt-2">
              <DiffViewBlock diff={diff} showTitle={false} className="mt-0 border-0 px-0" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
