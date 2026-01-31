'use client';

import { IconCheck, IconX, IconLoader2, IconEye, IconSearch } from '@tabler/icons-react';
import type { Message } from '@/lib/types/http';

type ReadFilePayload = {
  file_path?: string;
  offset?: number;
  limit?: number;
};

type CodeSearchPayload = {
  query?: string;
  pattern?: string;
  path?: string;
  glob?: string;
};

type NormalizedPayload = {
  kind?: string;
  read_file?: ReadFilePayload;
  code_search?: CodeSearchPayload;
};

type ToolReadMetadata = {
  tool_call_id?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  normalized?: NormalizedPayload;
};

export function ToolReadMessage({ comment }: { comment: Message }) {
  const metadata = comment.metadata as ToolReadMetadata | undefined;
  const status = metadata?.status;
  const normalized = metadata?.normalized;
  const kind = normalized?.kind;

  // Determine display based on kind
  const isCodeSearch = kind === 'code_search';
  const filePath = normalized?.read_file?.file_path;
  const searchPath = normalized?.code_search?.path;
  const searchPattern = normalized?.code_search?.glob || normalized?.code_search?.pattern;

  const isSuccess = status === 'complete';
  const Icon = isCodeSearch ? IconSearch : IconEye;

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

  // Display path - either file path for read or search path for code search
  const displayPath = isCodeSearch ? searchPath : filePath;
  const displayPattern = isCodeSearch ? searchPattern : null;

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full">
        <div className="flex-shrink-0 mt-0.5">
          <Icon className="h-4 w-4 text-muted-foreground" />
        </div>

        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <span className="font-mono text-xs">{comment.content}</span>
            {!isSuccess && getStatusIcon()}
            {displayPattern && (
              <span className="text-xs text-muted-foreground/60 font-mono truncate">{displayPattern}</span>
            )}
            {displayPath && (
              <span className="text-xs text-muted-foreground/60 truncate">{displayPath}</span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
