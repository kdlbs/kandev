"use client";

import { useCallback } from "react";
import { isArrowKey, KEY_SEQUENCES } from "@/lib/terminal/key-sequences";
import { sendShellArrow } from "@/lib/terminal/send-shell-input";
import { useShellKeySender } from "@/hooks/domains/session/use-shell-key-sender";
import {
  resolveVisualViewportPosition,
  useVisualViewportOffset,
} from "@/hooks/use-visual-viewport-offset";
import { refocusXtermTextarea } from "@/lib/terminal/refocus-xterm";
import { useShellModifiersStore } from "@/lib/terminal/shell-modifiers";
import { KEYS, KeybarButton, ModifierButton } from "./mobile-terminal-keybar-helpers";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { IconArrowsLeftRight } from "@tabler/icons-react";

export type MobileTerminalKeybarProps = {
  sessionId: string | null | undefined;
  visible: boolean;
  /** CSS length used as minimum bottom offset when the on-screen keyboard is closed (e.g., to clear the bottom nav). */
  baseBottomOffset?: string;
  /** Inline host terminals supply their own socket and xterm focus target. */
  inline?: boolean;
  onSend?: (data: string) => void;
  onFocus?: () => void;
};

/** Includes 44px touch targets, vertical padding, and the top border. */
export const KEYBAR_HEIGHT_PX = 58;
const INITIAL_NAVIGATION_KEY_COUNT = 7;

type MobileTerminalKeybarControlsProps = {
  ctrl: { latched: boolean; sticky: boolean };
  shift: { latched: boolean; sticky: boolean };
  onCtrlTap: () => void;
  onShiftTap: () => void;
  onSend: (data: string, key?: string) => void;
};

function MobileTerminalKeybarControls({
  ctrl,
  shift,
  onCtrlTap,
  onShiftTap,
  onSend,
}: MobileTerminalKeybarControlsProps) {
  const { t } = useTranslation();
  const renderKey = (key: (typeof KEYS)[number]) => (
    <KeybarButton
      key={key.id}
      id={key.id}
      ariaLabel={t(key.ariaLabelKey)}
      onTap={() => onSend(key.seq, key.id)}
    >
      {key.label}
    </KeybarButton>
  );

  return (
    <div className="flex w-full items-center gap-1">
      <div
        data-testid="mobile-terminal-keybar-scroller"
        className="flex min-w-0 flex-1 gap-0.5 overflow-x-auto px-2 py-1.5"
      >
        {KEYS.slice(0, INITIAL_NAVIGATION_KEY_COUNT).map(renderKey)}
        <ModifierButton
          id="ctrl"
          label={t("task:ctrl")}
          ariaLabel={t("task:control")}
          state={ctrl}
          onTap={onCtrlTap}
        />
        <ModifierButton
          id="shift"
          label={t("task:shift")}
          ariaLabel={t("task:shift")}
          state={shift}
          onTap={onShiftTap}
        />
        <KeybarButton
          id="ctrl-c"
          ariaLabel={t("task:controlC")}
          onTap={() => onSend(KEY_SEQUENCES.ctrlC)}
          variant="destructive"
        >
          ^C
        </KeybarButton>
        <KeybarButton
          id="ctrl-d"
          ariaLabel={t("task:controlD")}
          onTap={() => onSend(KEY_SEQUENCES.ctrlD)}
        >
          ^D
        </KeybarButton>
        {KEYS.slice(INITIAL_NAVIGATION_KEY_COUNT).map(renderKey)}
      </div>
      <span
        data-testid="keybar-scroll-cue"
        className="flex w-5 shrink-0 items-center justify-center text-muted-foreground"
        aria-hidden="true"
      >
        <IconArrowsLeftRight className="size-4" aria-hidden="true" />
      </span>
    </div>
  );
}

export function MobileTerminalKeybar({
  sessionId,
  visible,
  baseBottomOffset,
  inline = false,
  onSend,
  onFocus = refocusXtermTextarea,
}: MobileTerminalKeybarProps) {
  const send = useShellKeySender(sessionId);
  const { keyboardOpen, viewportBottom } = useVisualViewportOffset();
  const ctrl = useShellModifiersStore((s) => s.ctrl);
  const shift = useShellModifiersStore((s) => s.shift);
  const toggleCtrl = useShellModifiersStore((s) => s.toggleCtrl);
  const toggleShift = useShellModifiersStore((s) => s.toggleShift);

  const tapSend = useCallback(
    (data: string, key?: string) => {
      onFocus();
      if (!onSend && sessionId && key && isArrowKey(key)) sendShellArrow(sessionId, key);
      else (onSend ?? send)(data);
    },
    [onFocus, onSend, send, sessionId],
  );

  const onCtrlTap = useCallback(() => {
    onFocus();
    toggleCtrl();
  }, [onFocus, toggleCtrl]);

  const onShiftTap = useCallback(() => {
    onFocus();
    toggleShift();
  }, [onFocus, toggleShift]);

  if (!visible || !sessionId) return null;

  const position = resolveVisualViewportPosition({
    keyboardOpen,
    viewportBottom,
    barHeight: KEYBAR_HEIGHT_PX,
    baseBottomOffset,
  });

  return (
    <div
      data-testid="mobile-terminal-keybar"
      className={cn(
        "z-40 border-t border-border bg-background/95 backdrop-blur",
        inline ? "shrink-0" : "fixed left-0 right-0",
      )}
      style={{ ...(inline ? {} : position), height: `${KEYBAR_HEIGHT_PX}px` }}
    >
      <MobileTerminalKeybarControls
        ctrl={ctrl}
        shift={shift}
        onCtrlTap={onCtrlTap}
        onShiftTap={onShiftTap}
        onSend={tapSend}
      />
    </div>
  );
}
