"use client";

import { useCallback, useEffect, useRef } from "react";
import { IconLoader2, IconMicrophone, IconPlayerStopFilled } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import {
  useVoiceInput,
  type VoiceError,
  type VoiceInputState,
  type VoiceModelLoadState,
} from "@/hooks/use-voice-input";
import { useAppStore } from "@/components/state-provider";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useToast } from "@/components/toast-provider";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";

type VoiceInputButtonProps = {
  /** Inserts the recognized transcript at the current cursor position. */
  onTranscript: (text: string) => void;
  /** Called after a non-empty transcript was inserted, when auto-send is enabled. */
  onAutoSend?: () => void;
  /** Disable while the chat input itself is disabled (sending / starting / failed). */
  disabled?: boolean;
};

const TOOLTIP_BY_STATE: Record<VoiceInputState, string> = {
  idle: "Voice input",
  requesting: "Requesting microphone…",
  recording: "Stop recording",
  processing: "Transcribing…",
};

const ARIA_BY_STATE: Record<VoiceInputState, string> = {
  idle: "Start voice input",
  requesting: "Requesting microphone permission",
  recording: "Stop voice input",
  processing: "Transcribing voice input",
};

function ButtonIcon({
  state,
  modelLoad,
}: {
  state: VoiceInputState;
  modelLoad: VoiceModelLoadState;
}) {
  if (state === "processing" || state === "requesting" || modelLoad.state === "loading") {
    return <IconLoader2 className="h-4 w-4 animate-spin" />;
  }
  if (state === "recording") {
    return <IconPlayerStopFilled className="h-3.5 w-3.5" />;
  }
  return <IconMicrophone className="h-4 w-4" />;
}

function toastForError(toast: ReturnType<typeof useToast>["toast"], err: VoiceError) {
  if (err.code === "no-speech") {
    toast({ title: err.message });
    return;
  }
  toast({ title: err.message, variant: "error" });
}

// ── Activation handlers ──────────────────────────────────────────────────

function buildHoldHandlers(start: () => Promise<void>, stop: () => Promise<void>) {
  return {
    onPointerDown: (e: React.PointerEvent) => {
      e.preventDefault();
      void start();
    },
    onPointerUp: (e: React.PointerEvent) => {
      e.preventDefault();
      void stop();
    },
    onPointerLeave: () => void stop(),
    onPointerCancel: () => void stop(),
  };
}

function buildToggleHandler(
  state: VoiceInputState,
  start: () => Promise<void>,
  stop: () => Promise<void>,
) {
  return () => {
    if (state === "idle") void start();
    else if (state === "recording") void stop();
  };
}

// ── Hook composition ─────────────────────────────────────────────────────

function useAutoSendOnTranscript(
  baseOnTranscript: (text: string) => void,
  onAutoSend: (() => void) | undefined,
  enabled: boolean,
) {
  // Wrap onTranscript so we can defer auto-send until after the transcript
  // has been inserted. requestAnimationFrame keeps a clean separation between
  // the editor update and the submit handler, so the editor's onChange has
  // already flushed when submit reads from it.
  return useCallback(
    (text: string) => {
      baseOnTranscript(text);
      if (enabled && onAutoSend) requestAnimationFrame(onAutoSend);
    },
    [baseOnTranscript, onAutoSend, enabled],
  );
}

function useVoiceShortcut(
  enabled: boolean,
  state: VoiceInputState,
  start: () => Promise<void>,
  stop: () => Promise<void>,
) {
  const overrides = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const shortcut = getShortcut("VOICE_INPUT_TOGGLE", overrides);
  const stateRef = useRef(state);
  useEffect(() => {
    stateRef.current = state;
  }, [state]);
  const handler = useCallback(() => {
    if (stateRef.current === "idle") void start();
    else if (stateRef.current === "recording") void stop();
  }, [start, stop]);
  useKeyboardShortcut(shortcut, handler, { enabled });
}

// ── Unsupported fallback ────────────────────────────────────────────────

function buildUnsupportedReason(): string {
  if (typeof window === "undefined") return "Voice input is unavailable here.";
  if (!window.isSecureContext) {
    return "Voice input needs HTTPS. Open this site over https:// (or http://localhost) — most mobile browsers block microphone APIs on insecure origins.";
  }
  return "Voice input isn't supported in this browser. Try Chrome, Edge, or Safari 14.5+.";
}

function UnsupportedVoiceButton({ disabled }: { disabled?: boolean }) {
  const { toast } = useToast();
  const handleClick = () => {
    toast({
      title: "Voice input unavailable",
      description: buildUnsupportedReason(),
      variant: "error",
    });
  };
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="secondary"
          size="icon"
          aria-label="Voice input unavailable"
          data-testid="voice-input-button"
          data-state="unsupported"
          disabled={!!disabled}
          onClick={handleClick}
          className="h-7 w-7 rounded-full cursor-pointer text-muted-foreground/60"
        >
          <IconMicrophone className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Voice input unavailable — tap for details</TooltipContent>
    </Tooltip>
  );
}

// ── Component ────────────────────────────────────────────────────────────

export function VoiceInputButton({ onTranscript, onAutoSend, disabled }: VoiceInputButtonProps) {
  const enabled = useAppStore((s) => s.userSettings.voiceMode.enabled);
  // Render nothing — including no hook subscriptions — when the user has
  // disabled the feature in settings. Distinct from `!supported` (browser
  // limitation) which shows a tappable greyed icon. Done as a sub-component
  // so the unconditional hook count stays the same in the active path.
  if (!enabled) return null;
  return (
    <EnabledVoiceInputButton
      onTranscript={onTranscript}
      onAutoSend={onAutoSend}
      disabled={disabled}
    />
  );
}

function EnabledVoiceInputButton({ onTranscript, onAutoSend, disabled }: VoiceInputButtonProps) {
  const { toast } = useToast();
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
  const handleError = useCallback((err: VoiceError) => toastForError(toast, err), [toast]);
  const wrappedTranscript = useAutoSendOnTranscript(onTranscript, onAutoSend, voiceMode.autoSend);

  const { supported, state, modelLoad, start, stop, cancel } = useVoiceInput({
    onTranscript: wrappedTranscript,
    onError: handleError,
  });

  // If the chat input gets disabled mid-recording, cancel rather than leave
  // the mic indicator on. Hold-mode pointerup may not fire if focus moves.
  useEffect(() => {
    if (disabled && (state === "recording" || state === "requesting")) cancel();
  }, [disabled, state, cancel]);

  useVoiceShortcut(supported && !disabled, state, start, stop);

  // Always render the button — even when unsupported — so users can see it on
  // mobile and tap to learn why voice input isn't working (usually a missing
  // secure context, e.g. when reaching the dev server over LAN HTTP). Hiding
  // the button silently left mobile users with no discoverable feedback.
  if (!supported) return <UnsupportedVoiceButton disabled={disabled} />;

  const isRecording = state === "recording";
  const isBusy = state === "requesting" || state === "processing" || modelLoad.state === "loading";
  const holdMode = voiceMode.mode === "hold";

  const pointerHandlers = holdMode ? buildHoldHandlers(start, stop) : {};
  const onClick = holdMode ? undefined : buildToggleHandler(state, start, stop);

  // Styled to mirror SubmitButton (h-7 w-7 rounded-full primary fill) so the
  // two prominent input actions read as a pair on the right of the toolbar.
  // Recording flips to a destructive fill with a pulsing ring so the active
  // state is unmistakable even on mobile.
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="default"
          size="icon"
          aria-label={ARIA_BY_STATE[state]}
          aria-pressed={isRecording}
          data-testid="voice-input-button"
          data-state={state}
          data-mode={voiceMode.mode}
          disabled={!!disabled || (isBusy && state !== "recording")}
          onClick={onClick}
          {...pointerHandlers}
          className={cn(
            "h-7 w-7 rounded-full cursor-pointer relative select-none",
            isRecording && "bg-destructive text-destructive-foreground hover:bg-destructive/90",
          )}
        >
          <ButtonIcon state={state} modelLoad={modelLoad} />
          {isRecording && (
            <span
              aria-hidden
              className="absolute inset-0 rounded-full ring-2 ring-destructive/40 animate-pulse"
            />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {modelLoad.state === "loading"
          ? `Loading model… ${Math.round(modelLoad.progress * 100)}%`
          : `${TOOLTIP_BY_STATE[state]}${holdMode && state === "idle" ? " (hold)" : ""}`}
      </TooltipContent>
    </Tooltip>
  );
}
