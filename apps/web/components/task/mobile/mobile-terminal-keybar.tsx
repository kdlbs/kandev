"use client";

import { useCallback, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { KEY_SEQUENCES } from "@/lib/terminal/key-sequences";
import { useShellKeySender } from "@/hooks/domains/session/use-shell-key-sender";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";
import { refocusXtermTextarea } from "@/lib/terminal/refocus-xterm";
import { useShellModifiersStore, isActive } from "@/lib/terminal/shell-modifiers";

export type MobileTerminalKeybarProps = {
  sessionId: string | null | undefined;
  visible: boolean;
  /** CSS length used as minimum bottom offset when the on-screen keyboard is closed (e.g., to clear the bottom nav). */
  baseBottomOffset?: string;
};

/** Height of the bar in px (border-t + py-1.5 + h-8 button). Used by the mobile layout to pad the terminal so content doesn't hide behind the bar. */
export const KEYBAR_HEIGHT_PX = 48;

type KeyDef = {
  /** Stable id used for data-testid (e.g. `keybar-key-esc`). */
  id: string;
  /** Rendered button label. */
  label: ReactNode;
  /** Accessible label. */
  ariaLabel: string;
  /** Sequence emitted on tap. Ctrl/Shift transforms are applied downstream. */
  seq: string;
};

const KEYS: readonly KeyDef[] = [
  { id: "esc", label: "Esc", ariaLabel: "Escape", seq: KEY_SEQUENCES.esc },
  { id: "tab", label: "Tab", ariaLabel: "Tab", seq: KEY_SEQUENCES.tab },
  { id: "up", label: "↑", ariaLabel: "Arrow Up", seq: KEY_SEQUENCES.up },
  { id: "down", label: "↓", ariaLabel: "Arrow Down", seq: KEY_SEQUENCES.down },
  { id: "left", label: "←", ariaLabel: "Arrow Left", seq: KEY_SEQUENCES.left },
  { id: "right", label: "→", ariaLabel: "Arrow Right", seq: KEY_SEQUENCES.right },
  { id: "home", label: "Home", ariaLabel: "Home", seq: KEY_SEQUENCES.home },
  { id: "end", label: "End", ariaLabel: "End", seq: KEY_SEQUENCES.end },
  { id: "pageup", label: "PgUp", ariaLabel: "Page Up", seq: KEY_SEQUENCES.pageUp },
  { id: "pagedown", label: "PgDn", ariaLabel: "Page Down", seq: KEY_SEQUENCES.pageDown },
  { id: "pipe", label: "|", ariaLabel: "Pipe", seq: "|" },
  { id: "tilde", label: "~", ariaLabel: "Tilde", seq: "~" },
  { id: "slash", label: "/", ariaLabel: "Slash", seq: "/" },
  { id: "dash", label: "-", ariaLabel: "Dash", seq: "-" },
  { id: "underscore", label: "_", ariaLabel: "Underscore", seq: "_" },
];

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
        <ModifierButton
          id="ctrl"
          label="Ctrl"
          ariaLabel="Control"
          state={ctrl}
          onTap={onCtrlTap}
        />
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

type ModifierButtonProps = {
  id: string;
  label: string;
  ariaLabel: string;
  state: { latched: boolean; sticky: boolean };
  onTap: () => void;
};

function ModifierButton({ id, label, ariaLabel, state, onTap }: ModifierButtonProps) {
  return (
    <KeybarButton
      id={id}
      ariaLabel={ariaLabel}
      ariaPressed={isActive(state)}
      active={isActive(state)}
      sticky={state.sticky}
      onTap={onTap}
    >
      {label}
    </KeybarButton>
  );
}

type KeybarButtonProps = {
  id: string;
  ariaLabel: string;
  ariaPressed?: boolean;
  active?: boolean;
  sticky?: boolean;
  variant?: "default" | "destructive";
  onTap: () => void;
  children: ReactNode;
};

function KeybarButton({
  id,
  ariaLabel,
  ariaPressed,
  active,
  sticky,
  variant,
  onTap,
  children,
}: KeybarButtonProps) {
  return (
    <Button
      type="button"
      size="sm"
      variant={variant ?? (active ? "secondary" : "outline")}
      aria-label={ariaLabel}
      aria-pressed={ariaPressed}
      data-testid={`keybar-key-${id}`}
      data-sticky={sticky ? "true" : undefined}
      onPointerDown={(e) => e.preventDefault()}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onTap}
      style={{ touchAction: "manipulation" }}
      className={cn(
        "h-8 min-w-10 shrink-0 px-2 font-mono text-sm cursor-pointer",
        active && "ring-2 ring-primary/60",
        sticky && "ring-primary",
      )}
    >
      {children}
    </Button>
  );
}
