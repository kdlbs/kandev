"use client";

import { useCallback, useEffect, useRef, useSyncExternalStore } from "react";
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
import { whisperModelConfig } from "@/lib/voice/whisper-web-models";
import { VoiceModelLoadIndicator } from "./voice-model-load-indicator";

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

// Hold mode survives finger drift by claiming the pointer with
// setPointerCapture on pointerdown. Without capture, any movement that crosses
// the button's bounds (a common occurrence on touch — small finger shift,
// soft-keyboard reflow, OS gesture handoff) fires pointerleave and would stop
// recording mid-utterance. pointerleave is intentionally NOT wired here for
// the same reason. Capture is released on pointerup/pointercancel; both also
// trigger stop().
function safePointerCapture(target: Element, pointerId: number): void {
  try {
    target.setPointerCapture(pointerId);
  } catch {
    // setPointerCapture can throw InvalidPointerId on browsers that have
    // already released the implicit capture (older WebKit). Safe to ignore —
    // the worst case is the prior behaviour where hold could stop on drift.
  }
}

function safePointerRelease(target: Element, pointerId: number): void {
  try {
    // `hasPointerCapture` is in lib.dom but the runtime check guards against
    // ancient/headless environments that strip it (some jsdom + happy-dom
    // versions). If the method is present and reports no capture, skip the
    // release to avoid the no-op throw Safari occasionally fires.
    if (typeof target.hasPointerCapture === "function" && !target.hasPointerCapture(pointerId)) {
      return;
    }
    target.releasePointerCapture(pointerId);
  } catch {
    // ignore — Safari can throw if capture was already released.
  }
}

function buildHoldHandlers(start: () => Promise<void>, stop: () => Promise<void>) {
  return {
    onPointerDown: (e: React.PointerEvent) => {
      e.preventDefault();
      safePointerCapture(e.currentTarget, e.pointerId);
      void start();
    },
    onPointerUp: (e: React.PointerEvent) => {
      e.preventDefault();
      safePointerRelease(e.currentTarget, e.pointerId);
      void stop();
    },
    onPointerCancel: (e: React.PointerEvent) => {
      safePointerRelease(e.currentTarget, e.pointerId);
      void stop();
    },
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

// Subscribes to `(pointer: coarse)` so the component re-renders when the user
// docks/undocks an external pointer (Surface, iPad with Magic Keyboard). SSR
// returns false, matching desktop default — mobile-first hydration corrects on
// first client paint. Pattern mirrors `useResponsiveBreakpoint`.
function subscribeCoarsePointer(callback: () => void): () => void {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return () => {};
  const mql = window.matchMedia("(pointer: coarse)");
  mql.addEventListener("change", callback);
  return () => mql.removeEventListener("change", callback);
}

function getCoarsePointerSnapshot(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia("(pointer: coarse)").matches;
}

function getCoarsePointerServerSnapshot(): boolean {
  return false;
}

function useIsCoarsePointer(): boolean {
  return useSyncExternalStore(
    subscribeCoarsePointer,
    getCoarsePointerSnapshot,
    getCoarsePointerServerSnapshot,
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
          variant="ghost"
          size="icon"
          aria-label="Voice input unavailable"
          data-testid="voice-input-button"
          data-state="unsupported"
          disabled={!!disabled}
          onClick={handleClick}
          className="h-7 w-7 cursor-pointer text-muted-foreground/40 hover:text-muted-foreground"
        >
          <IconMicrophone className="h-4 w-4" />
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

// Hold-to-talk is unreliable on touch even with pointer capture: the platform
// reclaims the pointer for system gestures (back swipes, scroll-chains) and the
// user's stored preference becomes a trap. Coarse-pointer devices silently get
// toggle behaviour; persisted `voiceMode.mode` is left alone so docking a
// keyboard restores the user's choice without a save.
function resolveEffectiveMode(
  prefMode: "hold" | "toggle",
  isCoarsePointer: boolean,
): "hold" | "toggle" {
  return prefMode === "hold" && !isCoarsePointer ? "hold" : "toggle";
}

function resolveTooltip(args: {
  modelLoad: VoiceModelLoadState;
  modelLabel: string;
  state: VoiceInputState;
  holdMode: boolean;
}): string {
  const { modelLoad, modelLabel, state, holdMode } = args;
  if (modelLoad.state === "loading") {
    const pct = Number.isFinite(modelLoad.progress)
      ? Math.min(100, Math.max(0, Math.round(modelLoad.progress * 100)))
      : 0;
    return `Downloading ${modelLabel}… ${pct}%`;
  }
  return `${TOOLTIP_BY_STATE[state]}${holdMode && state === "idle" ? " (hold)" : ""}`;
}

type VoiceMicButtonProps = {
  state: VoiceInputState;
  modelLoad: VoiceModelLoadState;
  disabled?: boolean;
  storedMode: "hold" | "toggle";
  effectiveMode: "hold" | "toggle";
  isCoarsePointer: boolean;
  pointerHandlers: ReturnType<typeof buildHoldHandlers> | Record<string, never>;
  onClick: (() => void) | undefined;
};

// Ghost styled to match the other toolbar actions (attach, enhance) — voice
// is one secondary input action among several, not a co-primary alongside
// Submit. Recording flips to a tinted destructive look + pulsing ring so the
// active state stays unmistakable without shouting from a primary fill.
// Touch-input sizing bumps to 10×10 (40px) for an easier finger target
// without disturbing the desktop layout.
function VoiceMicButton({
  state,
  modelLoad,
  disabled,
  storedMode,
  effectiveMode,
  isCoarsePointer,
  pointerHandlers,
  onClick,
}: VoiceMicButtonProps) {
  const isRecording = state === "recording";
  const isBusy = state === "requesting" || state === "processing" || modelLoad.state === "loading";
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={ARIA_BY_STATE[state]}
      aria-pressed={isRecording}
      data-testid="voice-input-button"
      data-state={state}
      data-mode={storedMode}
      data-effective-mode={effectiveMode}
      disabled={!!disabled || (isBusy && state !== "recording")}
      onClick={onClick}
      {...pointerHandlers}
      className={cn(
        "cursor-pointer relative select-none text-muted-foreground hover:text-foreground hover:bg-muted/40",
        isCoarsePointer ? "h-10 w-10" : "h-7 w-7",
        isRecording &&
          "bg-destructive/15 text-destructive hover:bg-destructive/20 hover:text-destructive",
      )}
    >
      <ButtonIcon state={state} modelLoad={modelLoad} />
      {isRecording && (
        <span
          aria-hidden
          className="absolute inset-0 rounded-[inherit] ring-2 ring-destructive/40 animate-pulse"
        />
      )}
    </Button>
  );
}

function EnabledVoiceInputButton({ onTranscript, onAutoSend, disabled }: VoiceInputButtonProps) {
  const { toast } = useToast();
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
  const handleError = useCallback((err: VoiceError) => toastForError(toast, err), [toast]);
  const wrappedTranscript = useAutoSendOnTranscript(onTranscript, onAutoSend, voiceMode.autoSend);
  const isCoarsePointer = useIsCoarsePointer();

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

  const effectiveMode = resolveEffectiveMode(voiceMode.mode, isCoarsePointer);
  const holdMode = effectiveMode === "hold";
  const pointerHandlers = holdMode ? buildHoldHandlers(start, stop) : {};
  const onClick = holdMode ? undefined : buildToggleHandler(state, start, stop);
  const modelLabel = whisperModelConfig(voiceMode.whisperWebModel).label;
  const tooltipText = resolveTooltip({ modelLoad, modelLabel, state, holdMode });

  return (
    <div className="flex items-center gap-1.5">
      <VoiceModelLoadIndicator
        state={modelLoad.state}
        progress={modelLoad.progress}
        modelLabel={modelLabel}
      />
      <Tooltip>
        <TooltipTrigger asChild>
          <VoiceMicButton
            state={state}
            modelLoad={modelLoad}
            disabled={disabled}
            storedMode={voiceMode.mode}
            effectiveMode={effectiveMode}
            isCoarsePointer={isCoarsePointer}
            pointerHandlers={pointerHandlers}
            onClick={onClick}
          />
        </TooltipTrigger>
        <TooltipContent>{tooltipText}</TooltipContent>
      </Tooltip>
    </div>
  );
}
