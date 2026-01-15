'use client';

import { IconBrain } from '@tabler/icons-react';
import type { Comment } from '@/lib/types/http';
import type { RichMetadata } from '@/components/task/chat/types';

export function ThinkingMessage({ comment }: { comment: Comment }) {
  const metadata = comment.metadata as RichMetadata | undefined;
  const text = metadata?.thinking ?? comment.content;
  if (!text) return null;
  return (
    <div className="max-w-[85%] rounded-lg border border-border/50 bg-background/60 px-4 py-3 text-sm">
      <div className="flex items-center gap-2 text-muted-foreground mb-2 uppercase tracking-wide text-[11px]">
        <IconBrain className="h-3.5 w-3.5" />
        <span>Thinking</span>
      </div>
      <div className="whitespace-pre-wrap text-foreground/80">{text}</div>
    </div>
  );
}
