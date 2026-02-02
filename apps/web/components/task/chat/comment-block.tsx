'use client';

import { useState } from 'react';
import { Button } from '@kandev/ui/button';
import { Badge } from '@kandev/ui/badge';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@kandev/ui/collapsible';
import { IconChevronDown, IconChevronRight, IconX, IconMessage } from '@tabler/icons-react';
import { cn } from '@kandev/ui/lib/utils';
import type { DiffComment } from '@/lib/diff/types';
import { CommentDisplay } from '@/components/diff/comment-display';

interface CommentBlockProps {
  /** File path for the comments */
  filePath: string;
  /** Comments for this file */
  comments: DiffComment[];
  /** Callback to remove all comments for this file */
  onRemove: () => void;
  /** Callback to remove a specific comment */
  onRemoveComment: (commentId: string) => void;
  /** Callback when a comment is clicked (for jump-to-line) */
  onCommentClick?: (comment: DiffComment) => void;
  /** Additional class name */
  className?: string;
}

/**
 * Rich block component for embedding review comments in chat input.
 * Shows file name with comment count badge, expandable to see details.
 */
export function CommentBlock({
  filePath,
  comments,
  onRemove,
  onRemoveComment,
  onCommentClick,
  className,
}: CommentBlockProps) {
  const [isOpen, setIsOpen] = useState(false);

  // Get just the filename for display
  const fileName = filePath.split('/').pop() || filePath;

  if (comments.length === 0) {
    return null;
  }

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <div
        className={cn(
          'rounded-lg border border-border bg-muted/30',
          className
        )}
      >
        {/* Header */}
        <div className="flex items-center gap-2 px-2 py-1.5">
          <CollapsibleTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="h-5 w-5 cursor-pointer p-0"
            >
              {isOpen ? (
                <IconChevronDown className="h-3.5 w-3.5" />
              ) : (
                <IconChevronRight className="h-3.5 w-3.5" />
              )}
            </Button>
          </CollapsibleTrigger>

          <IconMessage className="h-3.5 w-3.5 text-blue-500" />

          <span
            className="min-w-0 flex-1 truncate text-xs font-medium"
            title={filePath}
          >
            {fileName}
          </span>

          <Badge
            variant="secondary"
            className="h-5 px-1.5 text-[10px]"
          >
            {comments.length}
          </Badge>

          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation();
              onRemove();
            }}
            className="h-5 w-5 cursor-pointer p-0 hover:text-destructive"
          >
            <IconX className="h-3 w-3" />
          </Button>
        </div>

        {/* Expanded content */}
        <CollapsibleContent>
          <div className="space-y-1.5 border-t border-border/50 px-2 py-2">
            {comments.map((comment) => (
              <div
                key={comment.id}
                onClick={() => onCommentClick?.(comment)}
                className={cn(
                  'rounded transition-colors',
                  onCommentClick && 'cursor-pointer hover:bg-muted/50'
                )}
              >
                <CommentDisplay
                  comment={comment}
                  compact
                  onDelete={() => onRemoveComment(comment.id)}
                />
              </div>
            ))}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  );
}

interface CommentBlocksContainerProps {
  /** Comments grouped by file path */
  commentsByFile: Record<string, DiffComment[]>;
  /** Session ID for removing comments */
  sessionId: string;
  /** Callback to remove comments for a file */
  onRemoveFile: (filePath: string) => void;
  /** Callback to remove a specific comment */
  onRemoveComment: (sessionId: string, filePath: string, commentId: string) => void;
  /** Callback when a comment is clicked (for jump-to-line) */
  onCommentClick?: (comment: DiffComment) => void;
  /** Additional class name */
  className?: string;
}

/**
 * Container for multiple CommentBlock components
 */
export function CommentBlocksContainer({
  commentsByFile,
  sessionId,
  onRemoveFile,
  onRemoveComment,
  onCommentClick,
  className,
}: CommentBlocksContainerProps) {
  const files = Object.keys(commentsByFile);

  if (files.length === 0) {
    return null;
  }

  return (
    <div className={cn('space-y-2', className)}>
      {files.map((filePath) => (
        <CommentBlock
          key={filePath}
          filePath={filePath}
          comments={commentsByFile[filePath]}
          onRemove={() => onRemoveFile(filePath)}
          onRemoveComment={(commentId) => onRemoveComment(sessionId, filePath, commentId)}
          onCommentClick={onCommentClick}
        />
      ))}
    </div>
  );
}
