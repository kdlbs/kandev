'use client';

import { useState } from 'react';
import {
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconCode,
  IconEdit,
  IconEye,
  IconFile,
  IconLoader2,
  IconSearch,
  IconTerminal2,
  IconX,
} from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import type { ToolCallMetadata } from '@/components/task/chat/types';

function getToolIcon(toolName: string | undefined, className: string) {
  const name = toolName?.toLowerCase() ?? '';
  if (name === 'edit' || name.includes('edit') || name.includes('replace') || name.includes('write') || name.includes('save')) {
    return <IconEdit className={className} />;
  }
  if (name === 'read' || name.includes('view') || name.includes('read')) {
    return <IconEye className={className} />;
  }
  if (name === 'search' || name.includes('search') || name.includes('find') || name.includes('retrieval')) {
    return <IconSearch className={className} />;
  }
  if (name === 'execute' || name.includes('terminal') || name.includes('exec') || name.includes('launch') || name.includes('process')) {
    return <IconTerminal2 className={className} />;
  }
  if (name === 'delete' || name === 'move' || name.includes('file') || name.includes('create')) {
    return <IconFile className={className} />;
  }
  return <IconCode className={className} />;
}

function getStatusIcon(status?: string) {
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
}

export function ToolCallMessage({ comment }: { comment: Message }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as ToolCallMetadata | undefined;

  const toolName = metadata?.tool_name ?? '';
  const title = metadata?.title ?? comment.content ?? 'Tool call';
  const status = metadata?.status;
  const args = metadata?.args;
  const result = metadata?.result;

  const toolIcon = getToolIcon(toolName, 'h-4 w-4 text-amber-600 dark:text-amber-400 flex-shrink-0');
  const hasDetails = args && Object.keys(args).length > 0;

  let filePath: string | undefined;
  const rawPath = args?.path ?? args?.file ?? args?.file_path;
  if (typeof rawPath === 'string') {
    filePath = rawPath;
  }
  if (!filePath && Array.isArray(args?.locations) && args.locations.length > 0) {
    const firstLoc = args.locations[0] as { path?: string } | undefined;
    if (firstLoc?.path) {
      filePath = firstLoc.path;
    }
  }

  return (
    <div className="rounded-md border border-border/40 bg-muted/20 overflow-hidden max-w-[85%]">
      <button
        type="button"
        onClick={() => hasDetails && setIsExpanded(!isExpanded)}
        className={cn(
          'w-full flex items-center gap-2 px-3 py-2 text-sm text-left',
          hasDetails && 'cursor-pointer hover:bg-muted/40 transition-colors'
        )}
        disabled={!hasDetails}
      >
        {toolIcon}
        <span className="flex-1 font-mono text-xs text-muted-foreground truncate">
          {title}
        </span>
        {filePath && (
          <span className="text-xs text-muted-foreground/60 truncate max-w-[200px]">
            {filePath}
          </span>
        )}
        {getStatusIcon(status)}
        {hasDetails && (
          isExpanded
            ? <IconChevronDown className="h-4 w-4 text-muted-foreground/50" />
            : <IconChevronRight className="h-4 w-4 text-muted-foreground/50" />
        )}
      </button>

      {isExpanded && hasDetails && (
        <div className="border-t border-border/30 bg-background/50 p-3 space-y-2">
          {args && Object.entries(args).map(([key, value]) => {
            const strValue = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
            const isLongValue = strValue.length > 100 || strValue.includes('\n');

            return (
              <div key={key} className="text-xs">
                <span className="font-medium text-muted-foreground">{key}:</span>
                {isLongValue ? (
                  <pre className="mt-1 p-2 bg-muted/50 rounded text-[11px] overflow-x-auto max-h-[200px] overflow-y-auto whitespace-pre-wrap break-all">
                    {strValue}
                  </pre>
                ) : (
                  <span className="ml-2 font-mono text-foreground/80">{strValue}</span>
                )}
              </div>
            );
          })}
          {result && (
            <div className="text-xs border-t border-border/30 pt-2 mt-2">
              <span className="font-medium text-muted-foreground">Result:</span>
              <pre className="mt-1 p-2 bg-muted/50 rounded text-[11px] overflow-x-auto max-h-[150px] overflow-y-auto whitespace-pre-wrap">
                {result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
