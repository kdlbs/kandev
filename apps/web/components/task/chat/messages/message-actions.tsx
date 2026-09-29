"use client";

import { useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import {
  IconCheck,
  IconCopy,
  IconCode,
  IconChevronLeft,
  IconChevronRight,
  IconEyeCode,
  IconHourglass,
  IconInfoCircle,
  IconStar,
  IconGitFork,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import { formatPromptDuration, messageTurnDurationSeconds } from "@/lib/prompt-history";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/app-state-types";
import type { Message, Turn } from "@/lib/types/http";
import {
  buildMessageDebugEntries,
  hasMessageDebugMetadata,
} from "@/components/task/chat/messages/message-debug-metadata";
import { formatMessageSessionConfig } from "@/components/task/chat/messages/message-session-config";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
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
import { Button } from "@kandev/ui/button";
import { forkConversation } from "@/lib/services/session-launch-service";

const ACTION_BUTTON_SIZE = "h-5 w-5 p-1";
const ACTION_BUTTON_HOVER = "hover:bg-muted rounded";
const ACTION_BUTTON_TRANSITION = "transition-colors duration-200";
const FORK_CONVERSATION_LABEL = "task:forkConversation";
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

/**
 * Debug dialog exposing a message's persisted and turn-derived metadata. The
 * entries area is a keyboard-focusable scroll region so long fields (e.g.
 * `turn_metadata`) stay reachable on every input modality.
 */
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
      <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden sm:max-w-2xl">
        <DialogHeader className="shrink-0">
          <DialogTitle>{t("task:messageMetadataTitle")}</DialogTitle>
        </DialogHeader>
        <div
          role="region"
          aria-label={t("task:messageMetadataEntries")}
          tabIndex={0}
          className="grid min-h-0 flex-1 gap-3 overflow-auto pr-1 outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
        >
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
  if (parseStrictRfc3339Timestamp(createdAt) === null) return null;
  const date = new Date(createdAt);
  const absoluteTime = date.toLocaleString();
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
function findMessageTurn(state: AppState, message: Message): Turn | null {
  const sessionId = message.session_id;
  const turnId = message.turn_id;
  if (!sessionId || !turnId) return null;
  return state.turns.bySession[sessionId]?.find((item) => item.id === turnId) ?? null;
}

function resolveMessageModelId(
  message: Message,
  turn: Turn | null,
  modelId?: string,
): string | undefined {
  return (message.metadata?.model ?? turn?.metadata?.model ?? modelId) as string | undefined;
}

function getMessageUsageMultiplier(
  state: AppState,
  sessionId: string,
  modelId: string | undefined,
) {
  return (
    state.sessionModels.bySessionId[sessionId]?.models.find((model) => model.modelId === modelId)
      ?.usageMultiplier ?? null
  );
}

function selectMessageTurnAndUsage(state: AppState, message: Message) {
  const sessionId = message.session_id ?? "";
  const turn = findMessageTurn(state, message);
  const currentModelId = state.sessionModels.bySessionId[sessionId]?.currentModelId;
  const modelId = resolveMessageModelId(message, turn, currentModelId);
  return {
    turn,
    usageMultiplier: getMessageUsageMultiplier(state, sessionId, modelId),
    nativeCodexForkEnabled: state.features?.codexAppServer ?? false,
  };
}

function useMessageTurnAndUsage(message: Message): {
  turn: Turn | null;
  usageMultiplier: string | null;
  nativeCodexForkEnabled: boolean;
} {
  return useAppStore(useShallow((state) => selectMessageTurnAndUsage(state, message)));
}

function ForkConversationAction({ message, turn }: { message: Message; turn: Turn }) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  const requestId = useRef<string | null>(null);

  const handleFork = async () => {
    if (!message.session_id || !message.task_id || !turn.completed_at || pending) return;
    requestId.current ??= crypto.randomUUID();
    setPending(true);
    setFailed(false);
    try {
      const response = await forkConversation({
        task_id: message.task_id,
        session_id: message.session_id,
        turn_id: turn.id,
        request_id: requestId.current,
      });
      setActiveSession(response.task_id, response.session_id);
      requestId.current = null;
      setOpen(false);
    } catch {
      setFailed(true);
    } finally {
      setPending(false);
    }
  };

  const trigger = (
    <button
      type="button"
      data-testid="fork-conversation-trigger"
      className={cn(
        "h-11 w-11 p-3 sm:h-5 sm:w-5 sm:p-1",
        ACTION_BUTTON_HOVER,
        ACTION_BUTTON_TRANSITION,
      )}
      title={t(FORK_CONVERSATION_LABEL)}
      aria-label={t(FORK_CONVERSATION_LABEL)}
    >
      <IconGitFork className="h-full w-full" />
    </button>
  );
  const confirmationActions = (
    <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
      <Button
        variant="outline"
        onClick={() => setOpen(false)}
        disabled={pending}
        className="min-h-11"
      >
        {t("common:cancel")}
      </Button>
      <Button
        onClick={() => void handleFork()}
        disabled={pending}
        className="min-h-11"
        data-testid="fork-conversation-confirm"
      >
        {t("task:forkConversationConfirm")}
      </Button>
    </div>
  );

  return usesTouchDrawer ? (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{t(FORK_CONVERSATION_LABEL)}</DrawerTitle>
          <DrawerDescription>{t("task:forkConversationSharedFiles")}</DrawerDescription>
        </DrawerHeader>
        <div className="px-4 pb-4">
          {failed && (
            <p role="alert" className="mb-3 text-sm text-destructive">
              {t("task:forkConversationFailed")}
            </p>
          )}
          {confirmationActions}
        </div>
      </DrawerContent>
    </Drawer>
  ) : (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t(FORK_CONVERSATION_LABEL)}</DialogTitle>
          <DialogDescription>{t("task:forkConversationSharedFiles")}</DialogDescription>
        </DialogHeader>
        {failed && (
          <p role="alert" className="text-sm text-destructive">
            {t("task:forkConversationFailed")}
          </p>
        )}
        <DialogFooter>{confirmationActions}</DialogFooter>
      </DialogContent>
    </Dialog>
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
  const { t } = useTranslation();
  const { copied, copy } = useCopyToClipboard();
  const { turn, usageMultiplier, nativeCodexForkEnabled } = useMessageTurnAndUsage(message);
  const usesTouchDrawer = useTouchDrawer();
  const durationSeconds = messageTurnDurationSeconds(message, turn);
  const sessionConfigText = formatMessageSessionConfig(message.metadata, turn?.metadata);
  const actionRowVisibility = usesTouchDrawer
    ? "opacity-100"
    : "opacity-100 sm:opacity-0 sm:group-hover:opacity-100";
  const handleCopy = async () => {
    await copy(message.content);
  };

  return (
    <div
      className={cn(
        "flex items-center gap-2 mt-2 focus-within:opacity-100 transition-opacity",
        actionRowVisibility,
      )}
    >
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
      {nativeCodexForkEnabled &&
        message.author_type === "agent" &&
        turn?.completed_at &&
        turn.metadata?.agent_type === "codex-app-server" && (
          <ForkConversationAction message={message} turn={turn} />
        )}
      {onToggleFavorite && <FavoriteButton isFavorite={isFavorite} onToggle={onToggleFavorite} />}
      <MessageMetaInfo
        showModel={showModel}
        sessionConfigText={sessionConfigText}
        showTimestamp={showTimestamp}
        createdAt={message.created_at}
      />
      {durationSeconds !== null && (
        <span
          data-testid="message-turn-duration"
          className="inline-flex items-center gap-1 whitespace-nowrap shrink-0 text-[10px] text-muted-foreground/60 font-mono"
        >
          <IconHourglass className="h-3 w-3 shrink-0" aria-hidden="true" />
          {formatPromptDuration(durationSeconds, {
            s: t("task:durationUnitSeconds"),
            m: t("task:durationUnitMinutes"),
            h: t("task:durationUnitHours"),
          })}
        </span>
      )}
    </div>
  );
}
