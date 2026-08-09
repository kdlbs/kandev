"use client";

import { useState } from "react";
import { useShallow } from "zustand/react/shallow";
import {
  IconCheck,
  IconCopy,
  IconCode,
  IconChevronLeft,
  IconChevronRight,
  IconEyeCode,
  IconInfoCircle,
  IconStar,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useAppStore } from "@/components/state-provider";
import type { Message, Turn } from "@/lib/types/http";
import {
  buildMessageDebugEntries,
  hasMessageDebugMetadata,
} from "@/components/task/chat/messages/message-debug-metadata";
import { formatMessageSessionConfig } from "@/components/task/chat/messages/message-session-config";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTranslation } from "react-i18next";

const ACTION_BUTTON_SIZE = "h-5 w-5 p-1";
const ACTION_BUTTON_HOVER = "hover:bg-muted rounded";
const ACTION_BUTTON_TRANSITION = "transition-colors duration-200";
// The JS value `null`, rendered verbatim in the debug-metadata dialog. Not copy.
const NULL_LITERAL = "null";

type MessageActionsProps = {
  message: Message;
  showCopy?: boolean;
  showTimestamp?: boolean;
  showRawToggle?: boolean;
  hasHiddenPrompts?: boolean;
  showNavigation?: boolean;
  showModel?: boolean;
  isRawView?: boolean;
  onToggleRaw?: () => void;
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
  hasPrev?: boolean;
  hasNext?: boolean;
  isFavorite?: boolean;
  onToggleFavorite?: () => void;
};

/** Renders the accessible favorite toggle in a message action row. */
function FavoriteButton({ isFavorite, onToggle }: { isFavorite: boolean; onToggle: () => void }) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={isFavorite}
      className={cn(
        ACTION_BUTTON_SIZE,
        ACTION_BUTTON_HOVER,
        ACTION_BUTTON_TRANSITION,
        "cursor-pointer",
        isFavorite && "text-yellow-500",
      )}
      title={isFavorite ? t("task:removeFromFavorites") : t("task:markAsFavorite")}
      aria-label={
        isFavorite ? t("task:removeMessageFromFavorites") : t("task:markMessageAsFavorite")
      }
    >
      <IconStar className={cn("h-full w-full", isFavorite && "fill-yellow-500")} />
    </button>
  );
}

function CopyButton({ copied, onCopy }: { copied: boolean; onCopy: () => void }) {
  const { t } = useTranslation();
  return (
    <button
      onClick={onCopy}
      className={cn(
        ACTION_BUTTON_SIZE,
        ACTION_BUTTON_HOVER,
        ACTION_BUTTON_TRANSITION,
        copied && "text-green-400",
      )}
      title={t("task:copyMessage")}
      aria-label={t("task:copyMessageToClipboard")}
    >
      {copied ? <IconCheck className="h-full w-full" /> : <IconCopy className="h-full w-full" />}
    </button>
  );
}

function NavigationButtons({
  hasPrev,
  hasNext,
  onNavigatePrev,
  onNavigateNext,
}: {
  hasPrev: boolean;
  hasNext: boolean;
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <button
        onClick={onNavigatePrev}
        disabled={!hasPrev}
        className={cn(
          ACTION_BUTTON_SIZE,
          ACTION_BUTTON_HOVER,
          ACTION_BUTTON_TRANSITION,
          "disabled:opacity-30 disabled:cursor-not-allowed",
        )}
        title={t("task:previousMessage")}
        aria-label={t("task:goToPreviousMessage")}
      >
        <IconChevronLeft className="h-full w-full" />
      </button>
      <button
        onClick={onNavigateNext}
        disabled={!hasNext}
        className={cn(
          ACTION_BUTTON_SIZE,
          ACTION_BUTTON_HOVER,
          ACTION_BUTTON_TRANSITION,
          "disabled:opacity-30 disabled:cursor-not-allowed",
        )}
        title={t("task:nextMessage")}
        aria-label={t("task:goToNextMessage")}
      >
        <IconChevronRight className="h-full w-full" />
      </button>
    </>
  );
}

function RawToggleButton({
  isRawView,
  onToggleRaw,
  hasHiddenPrompts,
}: {
  isRawView: boolean;
  onToggleRaw: () => void;
  hasHiddenPrompts?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <button
      onClick={onToggleRaw}
      className={cn(
        "flex items-center gap-0.5 rounded",
        ACTION_BUTTON_HOVER,
        ACTION_BUTTON_TRANSITION,
        hasHiddenPrompts ? "h-5 px-1 py-1" : ACTION_BUTTON_SIZE,
        isRawView && "bg-muted text-foreground",
      )}
      title={isRawView ? t("task:showFormatted") : t("task:showRawText")}
      aria-label={isRawView ? t("task:showFormattedMessage") : t("task:showRawText")}
    >
      <IconCode className="h-3 w-3" />
      {hasHiddenPrompts && <IconEyeCode className="h-3 w-3" />}
    </button>
  );
}

function MetadataValue({ value }: { value: unknown }) {
  if (value == null) return <span className="text-muted-foreground">{NULL_LITERAL}</span>;
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return <span className="font-mono text-muted-foreground">{String(value)}</span>;
  }
  return (
    <pre className="max-h-[48vh] overflow-auto rounded border bg-background p-3 text-[11px] leading-relaxed">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

function MessageDebugDialog({
  message,
  turn,
  usageMultiplier,
}: {
  message: Message;
  turn: Turn | null;
  usageMultiplier?: string | null;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const context = { usageMultiplier };
  if (!hasMessageDebugMetadata(message, turn, context)) return null;
  const entries = buildMessageDebugEntries(message, turn, context);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button
          className={cn(ACTION_BUTTON_SIZE, ACTION_BUTTON_HOVER, ACTION_BUTTON_TRANSITION)}
          title={t("task:messageMetadata")}
          aria-label={t("task:showMessageMetadata")}
        >
          <IconInfoCircle className="h-full w-full" />
        </button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("task:messageMetadataTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3 overflow-auto pr-1">
          {Object.entries(entries).map(([key, value]) => (
            <div key={key} className="grid gap-1">
              <div className="font-mono text-[10px] uppercase text-muted-foreground">{key}</div>
              <MetadataValue value={value} />
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function MessageTimestamp({ createdAt }: { createdAt: string }) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const absoluteTime = new Date(createdAt).toLocaleString();
  const timeEl = (
    <time
      dateTime={createdAt}
      title={absoluteTime}
      className="text-[10px] text-muted-foreground/60 font-mono"
    >
      {formatRelativeTime(createdAt)}
    </time>
  );

  if (!usesTouchDrawer) return timeEl;

  // Native `title` tooltips never fire on touch (no hover event), so coarse
  // pointers get a tap-to-open Drawer surfacing the same absolute time.
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <button
          type="button"
          data-testid="message-timestamp-trigger"
          className="cursor-pointer border-0 bg-transparent p-0 text-left"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={t("task:showFullTimestamp", { absoluteTime })}
        >
          {timeEl}
        </button>
      </DrawerTrigger>
      <DrawerContent data-testid="message-timestamp-drawer">
        <DrawerHeader>
          <DrawerTitle>{t("task:messageTime")}</DrawerTitle>
          <DrawerDescription>{absoluteTime}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

/** Selects the message turn and its model-specific usage multiplier. */
function useMessageTurnAndUsage(message: Message): {
  turn: Turn | null;
  usageMultiplier: string | null;
} {
  return useAppStore(
    useShallow((state) => {
      const turnId = message.turn_id;
      const turn =
        turnId && message.session_id
          ? (state.turns.bySession[message.session_id]?.find((item) => item.id === turnId) ?? null)
          : null;
      if (!message.session_id) return { turn, usageMultiplier: null };
      const sessionModels = state.sessionModels.bySessionId[message.session_id];
      const metadataModel = (message.metadata?.model ?? turn?.metadata?.model) as
        | string
        | undefined;
      const modelId = metadataModel ?? sessionModels?.currentModelId;
      const usageMultiplier =
        sessionModels?.models.find((model) => model.modelId === modelId)?.usageMultiplier ?? null;
      return { turn, usageMultiplier };
    }),
  );
}

function MessageMetaInfo({
  showModel,
  sessionConfigText,
  showTimestamp,
  createdAt,
}: {
  showModel: boolean;
  sessionConfigText: string | null;
  showTimestamp: boolean;
  createdAt: string;
}) {
  return (
    <>
      {showModel && sessionConfigText && (
        <span className="min-w-0 truncate text-[10px] text-muted-foreground/60 font-mono">
          {sessionConfigText}
        </span>
      )}
      {showTimestamp && <MessageTimestamp createdAt={createdAt} />}
    </>
  );
}

type ResolvedMessageActionsProps = {
  message: Message;
  showCopy: boolean;
  showTimestamp: boolean;
  showRawToggle: boolean;
  hasHiddenPrompts: boolean;
  showNavigation: boolean;
  showModel: boolean;
  isRawView: boolean;
  onToggleRaw?: () => void;
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
  hasPrev: boolean;
  hasNext: boolean;
  isFavorite: boolean;
  onToggleFavorite?: () => void;
};

/**
 * Resolves optional action props before rendering to keep JSX complexity low.
 */
function resolveMessageActionsProps(props: MessageActionsProps): ResolvedMessageActionsProps {
  return {
    message: props.message,
    showCopy: props.showCopy ?? true,
    showTimestamp: props.showTimestamp ?? true,
    showRawToggle: props.showRawToggle ?? true,
    hasHiddenPrompts: props.hasHiddenPrompts ?? false,
    showNavigation: props.showNavigation ?? false,
    showModel: props.showModel ?? false,
    isRawView: props.isRawView ?? false,
    onToggleRaw: props.onToggleRaw,
    onNavigatePrev: props.onNavigatePrev,
    onNavigateNext: props.onNavigateNext,
    hasPrev: props.hasPrev ?? false,
    hasNext: props.hasNext ?? false,
    isFavorite: props.isFavorite ?? false,
    onToggleFavorite: props.onToggleFavorite,
  };
}

/** Renders available actions and metadata for a chat message. */
export function MessageActions(props: MessageActionsProps) {
  const {
    message,
    showCopy,
    showTimestamp,
    showRawToggle,
    hasHiddenPrompts,
    showNavigation,
    showModel,
    isRawView,
    onToggleRaw,
    onNavigatePrev,
    onNavigateNext,
    hasPrev,
    hasNext,
    isFavorite,
    onToggleFavorite,
  } = resolveMessageActionsProps(props);
  const { copied, copy } = useCopyToClipboard();
  const { turn, usageMultiplier } = useMessageTurnAndUsage(message);
  const sessionConfigText = formatMessageSessionConfig(message.metadata, turn?.metadata);
  const handleCopy = async () => {
    await copy(message.content);
  };

  return (
    <div className="flex items-center gap-2 mt-2 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
      {showCopy && <CopyButton copied={copied} onCopy={handleCopy} />}
      {showRawToggle && onToggleRaw && (
        <RawToggleButton
          isRawView={isRawView}
          onToggleRaw={onToggleRaw}
          hasHiddenPrompts={hasHiddenPrompts}
        />
      )}
      {showNavigation && (
        <NavigationButtons
          hasPrev={hasPrev}
          hasNext={hasNext}
          onNavigatePrev={onNavigatePrev}
          onNavigateNext={onNavigateNext}
        />
      )}
      <MessageDebugDialog message={message} turn={turn} usageMultiplier={usageMultiplier} />
      {onToggleFavorite && <FavoriteButton isFavorite={isFavorite} onToggle={onToggleFavorite} />}
      <MessageMetaInfo
        showModel={showModel}
        sessionConfigText={sessionConfigText}
        showTimestamp={showTimestamp}
        createdAt={message.created_at}
      />
    </div>
  );
}
