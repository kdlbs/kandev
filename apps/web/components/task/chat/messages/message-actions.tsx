'use client';

import { IconCheck, IconCopy, IconCode, IconChevronLeft, IconChevronRight } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import { formatRelativeTime } from '@/lib/utils';
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard';
import { useAppStore } from '@/components/state-provider';
import type { Message } from '@/lib/types/http';

const ACTION_BUTTON_SIZE = 'h-5 w-5 p-1';
const ACTION_BUTTON_HOVER = 'hover:bg-muted rounded';
const ACTION_BUTTON_TRANSITION = 'transition-colors duration-200';

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

function useMessageModel(message: Message, showModel: boolean) {
  const sessionId = message.session_id;
  const messageModel = showModel ? (message.metadata as { model?: string } | undefined)?.model : undefined;
  const sessionModel = useAppStore((state) => {
    if (!showModel || messageModel || !sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    const snapshot = session?.agent_profile_snapshot as { model?: string } | null | undefined;
    return snapshot?.model ?? null;
  });
  return messageModel ?? sessionModel;
}

function CopyButton({ copied, onCopy }: { copied: boolean; onCopy: () => void }) {
  return (
    <button
      onClick={onCopy}
      className={cn(ACTION_BUTTON_SIZE, ACTION_BUTTON_HOVER, ACTION_BUTTON_TRANSITION, copied && 'text-green-400')}
      title="Copy message"
      aria-label="Copy message to clipboard"
    >
      {copied ? <IconCheck className="h-full w-full" /> : <IconCopy className="h-full w-full" />}
    </button>
  );
}

function NavigationButtons({ hasPrev, hasNext, onNavigatePrev, onNavigateNext }: {
  hasPrev: boolean; hasNext: boolean;
  onNavigatePrev?: () => void; onNavigateNext?: () => void;
}) {
  return (
    <>
      <button
        onClick={onNavigatePrev}
        disabled={!hasPrev}
        className={cn(ACTION_BUTTON_SIZE, ACTION_BUTTON_HOVER, ACTION_BUTTON_TRANSITION, 'disabled:opacity-30 disabled:cursor-not-allowed')}
        title="Previous message"
        aria-label="Go to previous message"
      >
        <IconChevronLeft className="h-full w-full" />
      </button>
      <button
        onClick={onNavigateNext}
        disabled={!hasNext}
        className={cn(ACTION_BUTTON_SIZE, ACTION_BUTTON_HOVER, ACTION_BUTTON_TRANSITION, 'disabled:opacity-30 disabled:cursor-not-allowed')}
        title="Next message"
        aria-label="Go to next message"
      >
        <IconChevronRight className="h-full w-full" />
      </button>
    </>
  );
}

function RawToggleButton({ isRawView, onToggleRaw }: { isRawView: boolean; onToggleRaw: () => void }) {
  return (
    <button
      onClick={onToggleRaw}
      className={cn(ACTION_BUTTON_SIZE, ACTION_BUTTON_HOVER, ACTION_BUTTON_TRANSITION, isRawView && 'bg-muted text-foreground')}
      title={isRawView ? 'Show formatted' : 'Show raw text'}
      aria-label={isRawView ? 'Show formatted message' : 'Show raw text'}
    >
      <IconCode className="h-full w-full" />
    </button>
  );
}

function MessageMetaInfo({ showModel, modelName, showTimestamp, createdAt }: {
  showModel: boolean; modelName: string | null | undefined;
  showTimestamp: boolean; createdAt: string;
}) {
  return (
    <>
      {showModel && modelName && <span className="text-[10px] text-muted-foreground/60 font-mono">{modelName}</span>}
      {showTimestamp && <span className="text-[10px] text-muted-foreground/60 font-mono">{formatRelativeTime(createdAt)}</span>}
    </>
  );
}

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
  const modelName = useMessageModel(message, showModel);
  const handleCopy = async () => { await copy(message.content); };

  return (
    <div className="flex items-center gap-2 mt-2 opacity-0 group-hover:opacity-100 transition-opacity">
      {showCopy && <CopyButton copied={copied} onCopy={handleCopy} />}
      {showRawToggle && onToggleRaw && <RawToggleButton isRawView={isRawView} onToggleRaw={onToggleRaw} />}
      {showNavigation && <NavigationButtons hasPrev={hasPrev} hasNext={hasNext} onNavigatePrev={onNavigatePrev} onNavigateNext={onNavigateNext} />}
      <MessageMetaInfo showModel={showModel} modelName={modelName} showTimestamp={showTimestamp} createdAt={message.created_at} />
    </div>
  );
}
