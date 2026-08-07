"use client";

import { type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { KEY_SEQUENCES } from "@/lib/terminal/key-sequences";
import { isActive } from "@/lib/terminal/shell-modifiers";

export type KeyDef = {
  /** Stable id used for data-testid (e.g. `keybar-key-esc`). */
  id: string;
  /** Rendered button label. A key cap or glyph, not translatable copy. */
  label: ReactNode;
  /**
   * Catalog key for the accessible label. A key holds the spoken name, which a
   * screen reader does read out, so it is copy — but this table is module
   * scope, so a resolved `t()` here would freeze at the boot locale.
   */
  ariaLabelKey: string;
  /** Sequence emitted on tap. Ctrl/Shift transforms are applied downstream. */
  seq: string;
};

export const KEYS: readonly KeyDef[] = [
  { id: "esc", label: "Esc", ariaLabelKey: "task:keyEscape", seq: KEY_SEQUENCES.esc },
  { id: "tab", label: "Tab", ariaLabelKey: "task:keyTab", seq: KEY_SEQUENCES.tab },
  { id: "up", label: "↑", ariaLabelKey: "task:keyArrowUp", seq: KEY_SEQUENCES.up },
  { id: "down", label: "↓", ariaLabelKey: "task:keyArrowDown", seq: KEY_SEQUENCES.down },
  { id: "left", label: "←", ariaLabelKey: "task:keyArrowLeft", seq: KEY_SEQUENCES.left },
  { id: "right", label: "→", ariaLabelKey: "task:keyArrowRight", seq: KEY_SEQUENCES.right },
  { id: "home", label: "Home", ariaLabelKey: "task:keyHome", seq: KEY_SEQUENCES.home },
  { id: "end", label: "End", ariaLabelKey: "task:keyEnd", seq: KEY_SEQUENCES.end },
  { id: "pageup", label: "PgUp", ariaLabelKey: "task:keyPageUp", seq: KEY_SEQUENCES.pageUp },
  { id: "pagedown", label: "PgDn", ariaLabelKey: "task:keyPageDown", seq: KEY_SEQUENCES.pageDown },
  { id: "pipe", label: "|", ariaLabelKey: "task:keyPipe", seq: "|" },
  { id: "tilde", label: "~", ariaLabelKey: "task:keyTilde", seq: "~" },
  { id: "slash", label: "/", ariaLabelKey: "task:keySlash", seq: "/" },
  { id: "dash", label: "-", ariaLabelKey: "task:keyDash", seq: "-" },
  { id: "underscore", label: "_", ariaLabelKey: "task:keyUnderscore", seq: "_" },
];

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

export function KeybarButton({
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

type ModifierButtonProps = {
  id: string;
  label: string;
  ariaLabel: string;
  state: { latched: boolean; sticky: boolean };
  onTap: () => void;
};

export function ModifierButton({ id, label, ariaLabel, state, onTap }: ModifierButtonProps) {
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
