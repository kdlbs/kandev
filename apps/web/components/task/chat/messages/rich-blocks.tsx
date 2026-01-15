'use client';

import { IconBrain, IconCode, IconListCheck } from '@tabler/icons-react';
import { DiffModeEnum, DiffView } from '@git-diff-view/react';
import { useTheme } from 'next-themes';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import type { DiffPayload, RichMetadata } from '@/components/task/chat/types';

function resolveDiffPayload(diff: unknown): DiffPayload | null {
  if (!diff) return null;
  if (typeof diff === 'string') {
    return null;
  }
  if (Array.isArray(diff)) {
    return { hunks: diff };
  }
  if (typeof diff === 'object' && diff !== null) {
    const candidate = diff as Partial<DiffPayload>;
    if (Array.isArray(candidate.hunks)) {
      return {
        hunks: candidate.hunks,
        oldFile: candidate.oldFile,
        newFile: candidate.newFile,
      };
    }
  }
  return null;
}

export function RichBlocks({ comment }: { comment: Message }) {
  const metadata = comment.metadata as RichMetadata | undefined;
  const { resolvedTheme } = useTheme();
  if (!metadata) return null;

  const todos = metadata.todos ?? [];
  const todoItems = todos
    .map((item) => (typeof item === 'string' ? { text: item, done: false } : item))
    .filter((item) => item.text);
  const diffPayload = resolveDiffPayload(metadata.diff);
  const diffText = typeof metadata.diff === 'string' ? metadata.diff : null;
  const diffTheme = resolvedTheme === 'dark' ? 'dark' : 'light';

  return (
    <>
      {metadata.thinking && (
        <div className="mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
            <IconBrain className="h-3.5 w-3.5" />
            <span>Thinking</span>
          </div>
          <div className="whitespace-pre-wrap text-foreground/80">{metadata.thinking}</div>
        </div>
      )}
      {todoItems.length > 0 && (
        <div className="mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
            <IconListCheck className="h-3.5 w-3.5" />
            <span>Todos</span>
          </div>
          <div className="space-y-1">
            {todoItems.map((todo) => (
              <div key={todo.text} className="flex items-center gap-2">
                <span
                  className={cn(
                    'h-1.5 w-1.5 rounded-full',
                    todo.done ? 'bg-green-500' : 'bg-muted-foreground/60'
                  )}
                />
                <span className={cn(todo.done && 'line-through text-muted-foreground')}>
                  {todo.text}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
      {diffPayload && (
        <div className="mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
            <IconCode className="h-3.5 w-3.5" />
            <span>Diff</span>
          </div>
          <DiffView
            data={{
              hunks: diffPayload.hunks,
              oldFile: diffPayload.oldFile ?? { fileName: 'before', fileLang: 'plaintext' },
              newFile: diffPayload.newFile ?? { fileName: 'after', fileLang: 'plaintext' },
            }}
            diffViewMode={DiffModeEnum.Unified}
            diffViewTheme={diffTheme}
          />
        </div>
      )}
      {!diffPayload && diffText && (
        <div className="mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
            <IconCode className="h-3.5 w-3.5" />
            <span>Diff</span>
          </div>
          <pre className="whitespace-pre-wrap break-words text-[11px] text-foreground/80">
            {diffText}
          </pre>
        </div>
      )}
    </>
  );
}
