"use client";

import { IconTerminal2, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { TerminalCloseInlineConfirmation } from "../terminal-close-inline-confirmation";

type MobileTerminalRowProps = {
  terminal: Terminal;
  isActive: boolean;
  isRunning: boolean;
  isConfirming: boolean;
  onSelect: (id: string) => void;
  onAskClose: (terminal: Terminal) => void;
  onCancelClose: () => void;
  onClose: () => void;
  onConfirmClose: () => void | Promise<void>;
};

export function MobileTerminalRow({
  terminal,
  isActive,
  isRunning,
  isConfirming,
  onSelect,
  onAskClose,
  onCancelClose,
  onClose,
  onConfirmClose,
}: MobileTerminalRowProps) {
  const { t } = useTranslation();
  return (
    <div
      data-testid={`mobile-terminal-row-${terminal.id}`}
      className={`flex min-h-11 items-center gap-2 border px-2 rounded-md cursor-pointer select-none ${
        isActive ? "border-primary/50 bg-card" : "border-transparent hover:bg-muted"
      }`}
    >
      <button
        type="button"
        className="flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-2 text-left"
        onClick={() => onSelect(terminal.id)}
      >
        <IconTerminal2 className="h-4 w-4 text-muted-foreground shrink-0" />
        <span className="text-sm truncate flex-1">{terminal.label}</span>
        {isRunning && !isConfirming && (
          <span className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 leading-none">
            {t("task:running")}
          </span>
        )}
      </button>
      {terminal.closable &&
        (isConfirming ? (
          <TerminalCloseInlineConfirmation
            density="touch"
            testId="mobile-terminal-close-confirmation"
            onCancel={onCancelClose}
            onClose={onClose}
            onConfirm={onConfirmClose}
          />
        ) : (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("task:close2", { label: terminal.label })}
            className="h-11 min-h-11 w-11 min-w-11 transition-[color,background-color,transform] duration-100 active:scale-[0.96]"
            onClick={() => onAskClose(terminal)}
          >
            <IconX className="h-4 w-4" aria-hidden="true" />
          </Button>
        ))}
    </div>
  );
}
