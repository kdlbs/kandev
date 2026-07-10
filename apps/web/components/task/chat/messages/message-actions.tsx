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
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";

const ACTION_BUTTON_SIZE = "h-5 w-5 p-1";
const ACTION_BUTTON_HOVER = "hover:bg-muted rounded";
const ACTION_BUTTON_TRANSITION = "transition-colors duration-200";

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
};

type SessionConfigSource = {
  model?: unknown;
  mode?: unknown;
  config_options?: unknown;
  configOptions?: unknown;
};

type MessageSessionConfig = {
  model: string | null;
  mode: string | null;
  configOptions: Array<{ label: string; value: string }>;
  configOptionsSet: boolean;
};

function stringValue(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value : null;
}

function isStringConfigEntry(entry: [string, unknown]): entry is [string, string] {
  const [key, optionValue] = entry;
  return key !== "model" && key !== "mode" && stringValue(optionValue) !== null;
}

function configOptionEntries(value: unknown): Array<{ label: string; value: string }> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return [];
  return Object.entries(value)
    .filter(isStringConfigEntry)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, optionValue]) => ({
      label: humanizeConfigKey(key),
      value: optionValue,
    }));
}

function humanizeConfigKey(key: string): string {
  return key
    .replace(/[_-]+/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .trim()
    .replace(/^./, (char) => char.toUpperCase());
}

function configFromSource(source: SessionConfigSource | null | undefined): MessageSessionConfig {
  if (!source) return emptySessionConfig();
  const configOptionsValue = source.config_options ?? source.configOptions;
  return {
    model: stringValue(source.model),
    mode: stringValue(source.mode),
    configOptions: configOptionEntries(configOptionsValue),
    configOptionsSet: isConfigOptionsObject(configOptionsValue),
  };
}

function emptySessionConfig(): MessageSessionConfig {
  return { model: null, mode: null, configOptions: [], configOptionsSet: false };
}

function isConfigOptionsObject(value: unknown): boolean {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function mergeSessionConfig(
  primary: MessageSessionConfig,
  fallback: MessageSessionConfig,
): MessageSessionConfig {
  return {
    model: primary.model ?? fallback.model,
    mode: primary.mode ?? fallback.mode,
    configOptions: mergedConfigOptions(primary, fallback),
    configOptionsSet: primary.configOptionsSet || fallback.configOptionsSet,
  };
}

function mergedConfigOptions(
  primary: MessageSessionConfig,
  fallback: MessageSessionConfig,
): MessageSessionConfig["configOptions"] {
  if (!primary.configOptionsSet) return fallback.configOptions;
  if (primary.configOptions.length === 0) return [];
  const optionsByLabel = new Map(fallback.configOptions.map((option) => [option.label, option]));
  for (const option of primary.configOptions) {
    optionsByLabel.set(option.label, option);
  }
  return Array.from(optionsByLabel.values()).sort((a, b) => a.label.localeCompare(b.label));
}

function formatSessionConfig(config: MessageSessionConfig): string | null {
  if (!config.model) return null;
  const details = formatSessionConfigDetails(config);
  return details ? `${config.model} · ${details}` : config.model;
}

function formatSessionConfigDetails(config: MessageSessionConfig): string | null {
  const details = [
    config.mode ? `Mode: ${config.mode}` : null,
    ...config.configOptions.map((option) => `${option.label}: ${option.value}`),
  ].filter(Boolean);
  return details.length > 0 ? details.join(" · ") : null;
}

function useMessageSessionConfigText(message: Message, showModel: boolean) {
  const sessionId = message.session_id;
  const messageConfig = showModel
    ? configFromSource(message.metadata as SessionConfigSource | undefined)
    : null;
  const [sessionConfigText, sessionDetailsText] = splitSessionConfigText(
    useSessionConfigText(sessionId, showModel),
  );
  if (!messageConfig?.model) return sessionConfigText || null;
  const messageDetails = formatSessionConfigDetails(messageConfig);
  const details = messageDetails ?? sessionDetailsText;
  return details ? `${messageConfig.model} · ${details}` : messageConfig.model;
}

function useSessionConfigText(sessionId: string | undefined, showModel: boolean) {
  return useAppStore((state) => {
    if (!showModel || !sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    const snapshot = configFromSource(
      session?.agent_profile_snapshot as SessionConfigSource | null | undefined,
    );
    const metadata = session?.metadata as Record<string, unknown> | null | undefined;
    const runtime = configFromSource(
      metadata?.runtime_config as SessionConfigSource | null | undefined,
    );
    const config = mergeSessionConfig(runtime, snapshot);
    return joinSessionConfigText(formatSessionConfig(config), formatSessionConfigDetails(config));
  });
}

function joinSessionConfigText(full: string | null, details: string | null): string {
  return JSON.stringify([full, details]);
}

function splitSessionConfigText(value: string | null): [string | null, string | null] {
  if (!value) return [null, null];
  const parsed = JSON.parse(value) as [unknown, unknown];
  const [full, details] = parsed;
  return [stringValue(full), stringValue(details)];
}

function CopyButton({ copied, onCopy }: { copied: boolean; onCopy: () => void }) {
  return (
    <button
      onClick={onCopy}
      className={cn(
        ACTION_BUTTON_SIZE,
        ACTION_BUTTON_HOVER,
        ACTION_BUTTON_TRANSITION,
        copied && "text-green-400",
      )}
      title="Copy message"
      aria-label="Copy message to clipboard"
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
        title="Previous message"
        aria-label="Go to previous message"
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
        title="Next message"
        aria-label="Go to next message"
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
      title={isRawView ? "Show formatted" : "Show raw text"}
      aria-label={isRawView ? "Show formatted message" : "Show raw text"}
    >
      <IconCode className="h-3 w-3" />
      {hasHiddenPrompts && <IconEyeCode className="h-3 w-3" />}
    </button>
  );
}

function MetadataValue({ value }: { value: unknown }) {
  if (value == null) return <span className="text-muted-foreground">null</span>;
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
  const [open, setOpen] = useState(false);
  const context = { usageMultiplier };
  if (!hasMessageDebugMetadata(message, turn, context)) return null;
  const entries = buildMessageDebugEntries(message, turn, context);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button
          className={cn(ACTION_BUTTON_SIZE, ACTION_BUTTON_HOVER, ACTION_BUTTON_TRANSITION)}
          title="Message metadata"
          aria-label="Show message metadata"
        >
          <IconInfoCircle className="h-full w-full" />
        </button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Message Metadata</DialogTitle>
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
      {showTimestamp && (
        <span className="text-[10px] text-muted-foreground/60 font-mono">
          {formatRelativeTime(createdAt)}
        </span>
      )}
    </>
  );
}

export function MessageActions({
  message,
  showCopy = true,
  showTimestamp = true,
  showRawToggle = true,
  hasHiddenPrompts = false,
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
  const sessionConfigText = useMessageSessionConfigText(message, showModel);
  const { turn, usageMultiplier } = useAppStore(
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
      <MessageMetaInfo
        showModel={showModel}
        sessionConfigText={sessionConfigText}
        showTimestamp={showTimestamp}
        createdAt={message.created_at}
      />
    </div>
  );
}
