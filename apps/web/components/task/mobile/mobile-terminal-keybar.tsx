"use client";

import { useCallback, useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { KEY_SEQUENCES, ctrlChord } from "@/lib/terminal/key-sequences";
import { useShellKeySender } from "@/hooks/domains/session/use-shell-key-sender";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";

export type MobileTerminalKeybarProps = {
  sessionId: string | null | undefined;
  visible: boolean;
  /** CSS length used as minimum bottom offset when the on-screen keyboard is closed (e.g., to clear the bottom nav). */
  baseBottomOffset?: string;
};

type KeyDef = {
  /** Stable id used for data-testid (e.g. `keybar-key-esc`). */
  id: string;
  /** Rendered button label. */
  label: ReactNode;
  /** Accessible label. */
  ariaLabel: string;
  /** Sequence emitted on plain tap. */
  seq: string;
  /** Character fed to ctrlChord when Ctrl is latched; null skips chording (use plain seq). */
  chordChar?: string;
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
  { id: "pipe", label: "|", ariaLabel: "Pipe", seq: "|", chordChar: "|" },
  { id: "tilde", label: "~", ariaLabel: "Tilde", seq: "~", chordChar: "~" },
  { id: "slash", label: "/", ariaLabel: "Slash", seq: "/", chordChar: "/" },
  { id: "dash", label: "-", ariaLabel: "Dash", seq: "-", chordChar: "-" },
  { id: "underscore", label: "_", ariaLabel: "Underscore", seq: "_", chordChar: "_" },
];

const LETTER_KEYS: readonly KeyDef[] = "abcdefghijklmnopqrstuvwxyz".split("").map((c) => ({
  id: `letter-${c}`,
  label: c,
  ariaLabel: `Letter ${c}`,
  seq: c,
  chordChar: c,
}));

export function MobileTerminalKeybar({
  sessionId,
  visible,
  baseBottomOffset,
}: MobileTerminalKeybarProps) {
  const send = useShellKeySender(sessionId);
  const { bottomOffset, keyboardOpen } = useVisualViewportOffset();
  const [ctrlLatched, setCtrlLatched] = useState(false);
  const [ctrlSticky, setCtrlSticky] = useState(false);

  const ctrlActive = ctrlLatched || ctrlSticky;

  const onCtrlTap = useCallback(() => {
    if (ctrlSticky) {
      setCtrlSticky(false);
      setCtrlLatched(false);
      return;
    }
    if (ctrlLatched) {
      setCtrlSticky(true);
      return;
    }
    setCtrlLatched(true);
  }, [ctrlLatched, ctrlSticky]);

  const onKeyTap = useCallback(
    (key: KeyDef) => {
      const chord = ctrlActive && key.chordChar ? ctrlChord(key.chordChar) : null;
      send(chord ?? key.seq);
      if (ctrlLatched && !ctrlSticky) setCtrlLatched(false);
    },
    [ctrlActive, ctrlLatched, ctrlSticky, send],
  );

  if (!visible || !sessionId) return null;

  const bottom = resolveBottom({ keyboardOpen, bottomOffset, baseBottomOffset });

  return (
    <div
      data-testid="mobile-terminal-keybar"
      className="fixed left-0 right-0 z-40 border-t border-border bg-background/95 backdrop-blur"
      style={{ bottom }}
    >
      <div className="flex w-full gap-1 overflow-x-auto px-2 py-1.5">
        <KeybarButton
          id="ctrl"
          ariaLabel="Control"
          ariaPressed={ctrlActive}
          onTap={onCtrlTap}
          active={ctrlActive}
          sticky={ctrlSticky}
        >
          Ctrl
        </KeybarButton>
        <KeybarButton
          id="ctrl-c"
          ariaLabel="Control C"
          onTap={() => send(KEY_SEQUENCES.ctrlC)}
          variant="destructive"
        >
          ^C
        </KeybarButton>
        <KeybarButton id="ctrl-d" ariaLabel="Control D" onTap={() => send(KEY_SEQUENCES.ctrlD)}>
          ^D
        </KeybarButton>
        {KEYS.map((key) => (
          <KeybarButton
            key={key.id}
            id={key.id}
            ariaLabel={key.ariaLabel}
            onTap={() => onKeyTap(key)}
          >
            {key.label}
          </KeybarButton>
        ))}
        {ctrlActive &&
          LETTER_KEYS.map((key) => (
            <KeybarButton
              key={key.id}
              id={key.id}
              ariaLabel={key.ariaLabel}
              onTap={() => onKeyTap(key)}
            >
              {key.label}
            </KeybarButton>
          ))}
      </div>
    </div>
  );
}

function resolveBottom({
  keyboardOpen,
  bottomOffset,
  baseBottomOffset,
}: {
  keyboardOpen: boolean;
  bottomOffset: number;
  baseBottomOffset: string | undefined;
}): string {
  if (keyboardOpen) return `${bottomOffset}px`;
  if (baseBottomOffset) return `calc(${baseBottomOffset} + env(safe-area-inset-bottom, 0px))`;
  return "env(safe-area-inset-bottom, 0px)";
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
      onClick={onTap}
      className={cn(
        "h-8 min-w-10 shrink-0 px-2 font-mono text-sm cursor-pointer",
        sticky && "ring-2 ring-primary/60",
      )}
    >
      {children}
    </Button>
  );
}
