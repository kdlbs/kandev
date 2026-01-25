'use client';

import { SplitSide } from '@git-diff-view/react';
import { Button } from '@kandev/ui/button';
import { Badge } from '@kandev/ui/badge';
import { IconTrash } from '@tabler/icons-react';
import type { DiffComment } from './types';

interface CommentRendererProps {
  comment: DiffComment;
  onDelete: (id: string) => void;
}

export function CommentRenderer({ comment, onDelete }: CommentRendererProps) {
  const isSingleLine = comment.startLine === comment.endLine;

  return (
    <div className="p-3 bg-amber-50 dark:bg-amber-950/30 border-t border-amber-200 dark:border-amber-800">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <div className="text-xs text-muted-foreground mb-1">
            {isSingleLine
              ? `Line ${comment.startLine}`
              : `Lines ${comment.startLine}-${comment.endLine}`}
            <Badge variant="outline" className="ml-2 text-xs">
              {comment.side === SplitSide.old ? 'old' : 'new'}
            </Badge>
          </div>
          <div className="text-sm whitespace-pre-wrap">{comment.content}</div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDelete(comment.id)}
          className="h-6 w-6 p-0 text-destructive hover:text-destructive shrink-0"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}
