'use client';

import { IconCheck, IconCopy, IconCode, IconChevronLeft, IconChevronRight } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import { formatRelativeTime } from '@/lib/utils';
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard';
import { useAppStore } from '@/components/state-provider';
import type { Message } from '@/lib/types/http';

type MessageActionsProps = {
  message: Message;
  showCopy?: boolean;
  showTimestamp?: boolean;
  showRawToggle?: boolean;
  showNavigation?: boolean;
  showModel?: boolean;
  isRawView?: boolean;
  onToggleRaw?: () => void;
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
  hasPrev?: boolean;
  hasNext?: boolean;
};

export function MessageActions({
  message,
  showCopy = true,
  showTimestamp = true,
  showRawToggle = true,
  showNavigation = false,
  showModel = false,
  isRawView = false,
  onToggleRaw,
  onNavigatePrev,
  onNavigateNext,
  hasPrev = false,
  hasNext = false,
}: MessageActionsProps) {
  const { copied, copy } = useCopyToClipboard();

  // Resolve model: prefer per-message metadata, fall back to session's agent profile snapshot
  const sessionId = message.session_id;
  const messageModel = showModel ? (message.metadata as { model?: string } | undefined)?.model : undefined;
  const sessionModel = useAppStore((state) => {
    if (!showModel || messageModel || !sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    const snapshot = session?.agent_profile_snapshot as { model?: string } | null | undefined;
    return snapshot?.model ?? null;
  });
  const modelName = messageModel ?? sessionModel;

  const handleCopy = async () => {
    await copy(message.content);
  };

  return (
    <div className="flex items-center gap-2 mt-2 opacity-0 group-hover:opacity-100 transition-opacity">
      {/* Copy button */}
      {showCopy && (
        <button
          onClick={handleCopy}
          className={cn(
            'h-5 w-5 p-1',
            'hover:bg-muted rounded',
            'transition-colors duration-200',
            copied && 'text-green-400',
          )}
          title="Copy message"
          aria-label="Copy message to clipboard"
        >
          {copied ? (
            <IconCheck className="h-full w-full" />
          ) : (
            <IconCopy className="h-full w-full" />
          )}
        </button>
      )}

      {/* Raw text toggle */}
      {showRawToggle && onToggleRaw && (
        <button
          onClick={onToggleRaw}
          className={cn(
            'h-5 w-5 p-1',
            'hover:bg-muted rounded',
            'transition-colors duration-200',
            isRawView && 'bg-muted text-foreground',
          )}
          title={isRawView ? 'Show formatted' : 'Show raw text'}
          aria-label={isRawView ? 'Show formatted message' : 'Show raw text'}
        >
          <IconCode className="h-full w-full" />
        </button>
      )}

      {/* Navigation buttons */}
      {showNavigation && (
        <>
          <button
            onClick={onNavigatePrev}
            disabled={!hasPrev}
            className={cn(
              'h-5 w-5 p-1',
              'hover:bg-muted rounded',
              'transition-colors duration-200',
              'disabled:opacity-30 disabled:cursor-not-allowed',
            )}
            title="Previous message"
            aria-label="Go to previous message"
          >
            <IconChevronLeft className="h-full w-full" />
          </button>
          <button
            onClick={onNavigateNext}
            disabled={!hasNext}
            className={cn(
              'h-5 w-5 p-1',
              'hover:bg-muted rounded',
              'transition-colors duration-200',
              'disabled:opacity-30 disabled:cursor-not-allowed',
            )}
            title="Next message"
            aria-label="Go to next message"
          >
            <IconChevronRight className="h-full w-full" />
          </button>
        </>
      )}

      {/* Model name */}
      {showModel && modelName && (
        <span className="text-[10px] text-muted-foreground/60 font-mono">
          {modelName}
        </span>
      )}

      {/* Timestamp */}
      {showTimestamp && (
        <span className="text-[10px] text-muted-foreground/60 font-mono">
          {formatRelativeTime(message.created_at)}
        </span>
      )}
    </div>
  );
}
