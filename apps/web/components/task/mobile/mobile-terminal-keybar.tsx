"use client";

import { useCallback } from "react";
import { KEY_SEQUENCES } from "@/lib/terminal/key-sequences";
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

export function MobileTerminalKeybar({
  sessionId,
  visible,
  baseBottomOffset,
  inline = false,
  onSend,
  onFocus = refocusXtermTextarea,
}: MobileTerminalKeybarProps) {
  const { t } = useTranslation();
  const send = useShellKeySender(sessionId);
  const { keyboardOpen, viewportBottom } = useVisualViewportOffset();
  const ctrl = useShellModifiersStore((s) => s.ctrl);
  const shift = useShellModifiersStore((s) => s.shift);
  const toggleCtrl = useShellModifiersStore((s) => s.toggleCtrl);
  const toggleShift = useShellModifiersStore((s) => s.toggleShift);

  const tapSend = useCallback(
    (data: string) => {
      onFocus();
      (onSend ?? send)(data);
    },
    [onFocus, onSend, send],
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
      <div className="flex w-full gap-1 overflow-x-auto px-2 py-1.5">
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
          onTap={() => tapSend(KEY_SEQUENCES.ctrlC)}
          variant="destructive"
        >
          ^C
        </KeybarButton>
        <KeybarButton
          id="ctrl-d"
          ariaLabel={t("task:controlD")}
          onTap={() => tapSend(KEY_SEQUENCES.ctrlD)}
        >
          ^D
        </KeybarButton>
        {KEYS.map((key) => (
          <KeybarButton
            key={key.id}
            id={key.id}
            ariaLabel={t(key.ariaLabelKey)}
            onTap={() => tapSend(key.seq)}
          >
            {key.label}
          </KeybarButton>
        ))}
      </div>
    </div>
  );
}
