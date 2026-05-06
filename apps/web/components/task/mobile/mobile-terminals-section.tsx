"use client";

import { memo, useCallback, useState } from "react";
import { IconPlus, IconTerminal2, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { useAppStore } from "@/components/state-provider";
import { useMobileTerminals } from "@/hooks/domains/session/use-mobile-terminals";
import { stopUserShell } from "@/lib/api/domains/user-shell-api";
import { useUserShells } from "@/hooks/domains/session/use-user-shells";
import { MobilePillButton } from "./mobile-pill-button";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

function TerminalRow({
  terminal,
  isActive,
  isRunning,
  onSelect,
  onAskClose,
}: {
  terminal: Terminal;
  isActive: boolean;
  isRunning: boolean;
  onSelect: (id: string) => void;
  onAskClose: (terminal: Terminal) => void;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSelect(terminal.id)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(terminal.id);
        }
      }}
      data-testid={`mobile-terminal-row-${terminal.id}`}
      className={`flex items-center gap-2 px-2 py-2 rounded-md cursor-pointer select-none ${
        isActive ? "bg-accent" : "hover:bg-accent/50"
      }`}
    >
      <IconTerminal2 className="h-4 w-4 text-muted-foreground shrink-0" />
      <span className="text-sm truncate flex-1">{terminal.label}</span>
      {isRunning && (
        <span className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 leading-none">
          running
        </span>
      )}
      {terminal.closable && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`Close ${terminal.label}`}
          className="cursor-pointer h-7 w-7"
          onClick={(e) => {
            e.stopPropagation();
            onAskClose(terminal);
          }}
        >
          <IconX className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}

function CloseTerminalConfirmDialog({
  terminal,
  onOpenChange,
  onConfirm,
}: {
  terminal: Terminal | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={terminal !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Close terminal?</AlertDialogTitle>
          <AlertDialogDescription>
            {`This stops the “${terminal?.label ?? ""}” shell and any process it's running.`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              onOpenChange(false);
              onConfirm();
            }}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Close terminal
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

const MobileTerminalsList = memo(function MobileTerminalsList({
  sessionId,
  onClose,
}: {
  sessionId: string | null;
  onClose: () => void;
}) {
  const { terminals, terminalTabValue, addTerminal, removeTerminal, environmentId } =
    useMobileTerminals(sessionId);
  const { shells } = useUserShells(environmentId);
  const setRightPanelActiveTab = useAppStore((s) => s.setRightPanelActiveTab);
  const [pendingClose, setPendingClose] = useState<Terminal | null>(null);

  const isShellRunning = useCallback(
    (id: string) => shells.find((s) => s.terminalId === id)?.running ?? false,
    [shells],
  );

  const handleSelect = useCallback(
    (id: string) => {
      if (sessionId) setRightPanelActiveTab(sessionId, id);
      onClose();
    },
    [sessionId, setRightPanelActiveTab, onClose],
  );

  const handleConfirmClose = useCallback(() => {
    const t = pendingClose;
    if (!t) return;
    removeTerminal(t.id);
    if (environmentId) {
      stopUserShell(environmentId, t.id).catch((err) => {
        console.error("Failed to stop terminal:", err);
      });
    }
    setPendingClose(null);
  }, [pendingClose, removeTerminal, environmentId]);

  const handleAskClose = useCallback((terminal: Terminal) => {
    setPendingClose(terminal);
  }, []);

  if (!sessionId) {
    return (
      <div className="text-xs text-muted-foreground px-2 py-6 text-center">No active session</div>
    );
  }

  return (
    <div className="flex flex-col gap-2 px-1">
      <div className="flex items-center justify-between px-1">
        <span className="text-xs font-medium text-muted-foreground">
          {terminals.length} terminal{terminals.length === 1 ? "" : "s"}
        </span>
        <Button
          size="sm"
          variant="outline"
          className="h-7 gap-1 cursor-pointer"
          onClick={addTerminal}
          data-testid="mobile-add-terminal"
        >
          <IconPlus className="h-4 w-4" />
          New terminal
        </Button>
      </div>
      <div className="flex flex-col gap-0.5">
        {terminals.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            No terminals yet.
          </div>
        )}
        {terminals.map((t) => (
          <TerminalRow
            key={t.id}
            terminal={t}
            isActive={t.id === terminalTabValue}
            isRunning={isShellRunning(t.id)}
            onSelect={handleSelect}
            onAskClose={handleAskClose}
          />
        ))}
      </div>
      <CloseTerminalConfirmDialog
        terminal={pendingClose}
        onOpenChange={(open) => {
          if (!open) setPendingClose(null);
        }}
        onConfirm={handleConfirmClose}
      />
    </div>
  );
});

function useActiveTerminalPillLabel(sessionId: string | null): {
  label: string;
  count: string | undefined;
} {
  const { terminals, terminalTabValue } = useMobileTerminals(sessionId);
  const activeIdx = terminals.findIndex((t) => t.id === terminalTabValue);
  const idx = activeIdx >= 0 ? activeIdx : 0;
  const active = terminals[idx];
  const total = terminals.length;
  let count: string | undefined;
  if (total > 1) count = `${idx + 1}/${total}`;
  return { label: active?.label ?? "Terminal", count };
}

export const MobileTerminalsPicker = memo(function MobileTerminalsPicker({
  sessionId,
  compact,
  fullWidth,
}: {
  sessionId: string | null;
  compact?: boolean;
  fullWidth?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const { label, count } = useActiveTerminalPillLabel(sessionId);
  if (!sessionId) return null;
  return (
    <>
      <MobilePillButton
        label={label}
        count={count}
        compact={compact}
        fullWidth={fullWidth}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="mobile-terminals-pill"
        ariaLabel={`Active terminal: ${label}. Tap to switch.`}
      />
      <MobilePickerSheet open={open} onOpenChange={setOpen} title="Terminals">
        <MobileTerminalsList sessionId={sessionId} onClose={() => setOpen(false)} />
      </MobilePickerSheet>
    </>
  );
});
