'use client';

import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from 'react';
import { IconX, IconClock, IconEdit, IconCheck } from '@tabler/icons-react';
import { Button } from '@kandev/ui';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { Textarea } from '@kandev/ui/textarea';

export type QueuedMessageIndicatorHandle = {
  startEdit: () => void;
};

type QueuedMessageIndicatorProps = {
  content: string;
  onCancel: () => void;
  onUpdate: (content: string) => Promise<void>;
  isVisible: boolean;
  onEditComplete?: () => void;
};

export const QueuedMessageIndicator = forwardRef<QueuedMessageIndicatorHandle, QueuedMessageIndicatorProps>(
  function QueuedMessageIndicator(
    { content, onCancel, onUpdate, isVisible, onEditComplete },
    ref
  ) {
    const [isEditing, setIsEditing] = useState(false);
    const [editValue, setEditValue] = useState(content);
    const [isSaving, setIsSaving] = useState(false);
    const textareaRef = useRef<HTMLTextAreaElement>(null);

    // Update edit value when content changes from outside (WebSocket updates)
    useEffect(() => {
      if (!isEditing) {
        setEditValue(content);
      }
    }, [content, isEditing]);

    // Auto-focus textarea when entering edit mode
    useEffect(() => {
      if (isEditing && textareaRef.current) {
        textareaRef.current.focus();
        // Place cursor at end
        textareaRef.current.setSelectionRange(
          textareaRef.current.value.length,
          textareaRef.current.value.length
        );
      }
    }, [isEditing]);

    const startEdit = useCallback(() => {
      setEditValue(content);
      setIsEditing(true);
    }, [content]);

    const handleSave = useCallback(async () => {
      const trimmed = editValue.trim();
      if (!trimmed || trimmed === content) {
        setIsEditing(false);
        onEditComplete?.();
        return;
      }

      setIsSaving(true);
      try {
        await onUpdate(trimmed);
        setIsEditing(false);
        onEditComplete?.();
      } catch (error) {
        console.error('Failed to update queued message:', error);
      } finally {
        setIsSaving(false);
      }
    }, [editValue, content, onUpdate, onEditComplete]);

    const handleCancel = useCallback(() => {
      setEditValue(content);
      setIsEditing(false);
      onEditComplete?.();
    }, [content, onEditComplete]);

    const handleKeyDown = useCallback(
      (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
        if (event.key === 'Escape') {
          event.preventDefault();
          handleCancel();
        } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
          event.preventDefault();
          handleSave();
        }
      },
      [handleCancel, handleSave]
    );

    useImperativeHandle(ref, () => ({
      startEdit,
    }), [startEdit]);

    if (!isVisible) return null;

    // Truncate content for display (not in edit mode)
    const displayContent = content.length > 80 ? content.substring(0, 80) + '...' : content;

    return (
      <div
        className={cn(
          'bg-blue-50 dark:bg-blue-950/20 border border-blue-200 dark:border-blue-800/50',
          'rounded-lg text-sm'
        )}
      >
        {isEditing ? (
          <div className="p-2 space-y-2">
            <Textarea
              ref={textareaRef}
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
              onKeyDown={handleKeyDown}
              className={cn(
                'min-h-[60px] resize-none',
                'bg-white dark:bg-gray-900',
                'border-blue-300 dark:border-blue-700',
                'focus:ring-blue-500 focus:border-blue-500'
              )}
              placeholder="Enter message content..."
              disabled={isSaving}
            />
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="default"
                onClick={handleSave}
                disabled={isSaving || !editValue.trim()}
                className="h-7"
              >
                <IconCheck className="h-3.5 w-3.5 mr-1" />
                Save
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={handleCancel}
                disabled={isSaving}
                className="h-7"
              >
                Cancel
              </Button>
              <span className="text-xs text-muted-foreground ml-auto">
                Press Esc to cancel, Cmd+Enter to save
              </span>
            </div>
          </div>
        ) : (
          <div className="flex items-center gap-2 px-3 py-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex-shrink-0">
                  <IconClock className="h-4 w-4 text-blue-600 dark:text-blue-400" />
                </div>
              </TooltipTrigger>
              <TooltipContent side="top">
                Message queued - will execute when agent completes
              </TooltipContent>
            </Tooltip>
            <div className="flex-1 min-w-0 text-blue-700 dark:text-blue-300 truncate">
              {displayContent}
            </div>
            <div className="flex items-center gap-1 flex-shrink-0">
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 hover:bg-blue-100 dark:hover:bg-blue-900"
                onClick={startEdit}
                title="Edit message"
              >
                <IconEdit className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 hover:bg-blue-100 dark:hover:bg-blue-900"
                onClick={onCancel}
                title="Cancel queued message"
              >
                <IconX className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}
      </div>
    );
  }
);
