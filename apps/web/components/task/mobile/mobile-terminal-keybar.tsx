"use client";

import { useCallback } from "react";
import { KEY_SEQUENCES } from "@/lib/terminal/key-sequences";
import { useShellKeySender } from "@/hooks/domains/session/use-shell-key-sender";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";
import { refocusXtermTextarea } from "@/lib/terminal/refocus-xterm";
import { useShellModifiersStore } from "@/lib/terminal/shell-modifiers";
import { KEYS, KeybarButton, ModifierButton } from "./mobile-terminal-keybar-helpers";

export type MobileTerminalKeybarProps = {
  sessionId: string | null | undefined;
  visible: boolean;
  /** CSS length used as minimum bottom offset when the on-screen keyboard is closed (e.g., to clear the bottom nav). */
  baseBottomOffset?: string;
};

/** Height of the bar in px (border-t + py-1.5 + h-8 button). Used by the mobile layout to pad the terminal so content doesn't hide behind the bar. */
export const KEYBAR_HEIGHT_PX = 48;

export function MobileTerminalKeybar({
  sessionId,
  visible,
  baseBottomOffset,
}: MobileTerminalKeybarProps) {
  const send = useShellKeySender(sessionId);
  const { keyboardOpen, viewportBottom } = useVisualViewportOffset();
  const ctrl = useShellModifiersStore((s) => s.ctrl);
  const shift = useShellModifiersStore((s) => s.shift);
  const toggleCtrl = useShellModifiersStore((s) => s.toggleCtrl);
  const toggleShift = useShellModifiersStore((s) => s.toggleShift);

  const tapSend = useCallback(
    (data: string) => {
      refocusXtermTextarea();
      send(data);
    },
    [send],
  );

  const onCtrlTap = useCallback(() => {
    refocusXtermTextarea();
    toggleCtrl();
  }, [toggleCtrl]);

  const onShiftTap = useCallback(() => {
    refocusXtermTextarea();
    toggleShift();
  }, [toggleShift]);

  if (!visible || !sessionId) return null;

  const position = resolvePosition({ keyboardOpen, viewportBottom, baseBottomOffset });

  return (
    <div
      data-testid="mobile-terminal-keybar"
      className="fixed left-0 right-0 z-40 border-t border-border bg-background/95 backdrop-blur"
      style={{ ...position, height: `${KEYBAR_HEIGHT_PX}px` }}
    >
      <div className="flex w-full gap-1 overflow-x-auto px-2 py-1.5">
        <ModifierButton id="ctrl" label="Ctrl" ariaLabel="Control" state={ctrl} onTap={onCtrlTap} />
        <ModifierButton
          id="shift"
          label="Shift"
          ariaLabel="Shift"
          state={shift}
          onTap={onShiftTap}
        />
        <KeybarButton
          id="ctrl-c"
          ariaLabel="Control C"
          onTap={() => tapSend(KEY_SEQUENCES.ctrlC)}
          variant="destructive"
        >
          ^C
        </KeybarButton>
        <KeybarButton id="ctrl-d" ariaLabel="Control D" onTap={() => tapSend(KEY_SEQUENCES.ctrlD)}>
          ^D
        </KeybarButton>
        {KEYS.map((key) => (
          <KeybarButton
            key={key.id}
            id={key.id}
            ariaLabel={key.ariaLabel}
            onTap={() => tapSend(key.seq)}
          >
            {key.label}
          </KeybarButton>
        ))}
      </div>
    </div>
  );
}

function resolvePosition({
  keyboardOpen,
  viewportBottom,
  baseBottomOffset,
}: {
  keyboardOpen: boolean;
  viewportBottom: number;
  baseBottomOffset: string | undefined;
}): React.CSSProperties {
  // iOS Safari drifts fixed elements positioned via `bottom` while the visual
  // viewport scrolls with the keyboard up. Anchoring via `top` tied to the
  // visual viewport's bottom edge stays glued to the keyboard.
  if (keyboardOpen) {
    return { top: `${viewportBottom - KEYBAR_HEIGHT_PX}px`, bottom: "auto" };
  }
  const base = baseBottomOffset
    ? `calc(${baseBottomOffset} + env(safe-area-inset-bottom, 0px))`
    : "env(safe-area-inset-bottom, 0px)";
  return { bottom: base };
}
