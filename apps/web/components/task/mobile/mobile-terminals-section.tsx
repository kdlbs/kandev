"use client";

import { memo, useCallback, useState } from "react";
import { IconPlus, IconTerminal2, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { stopUserShell } from "@/lib/api/domains/user-shell-api";
import { shouldConfirmTerminalClose } from "@/lib/terminal/terminal-busy-registry";
import { useUserShells } from "@/hooks/domains/session/use-user-shells";
import { releaseAutoCreatedEnvironment } from "@/hooks/domains/session/use-mobile-terminals";
import { CloseTerminalConfirmDialog } from "../close-terminal-confirm-dialog";
import { MobilePillButton } from "./mobile-pill-button";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import { useMobileTerminalsContext } from "./mobile-terminals-context";
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

type CloseHandlerArgs = {
  sessionId: string | null;
  environmentId: string | null;
  taskId: string | null;
  terminals: Terminal[];
  terminalTabValue: string;
  removeTerminal: (id: string) => void;
  setRightPanelActiveTab: (sessionId: string, tab: string) => void;
};

function useTerminalCloseHandler({
  sessionId,
  environmentId,
  taskId,
  terminals,
  terminalTabValue,
  removeTerminal,
  setRightPanelActiveTab,
}: CloseHandlerArgs) {
  const [pendingClose, setPendingClose] = useState<Terminal | null>(null);

  const closeTerminal = useCallback(
    async (t: Terminal) => {
      if (!sessionId) return;
      try {
        if (environmentId) await stopUserShell(environmentId, t.id, taskId ?? undefined);
        if (terminalTabValue === t.id) {
          const next = terminals.find((row) => row.id !== t.id);
          if (next) setRightPanelActiveTab(sessionId, next.id);
        }
        removeTerminal(t.id);
        if (environmentId && terminals.length <= 1) {
          releaseAutoCreatedEnvironment(environmentId);
        }
        setPendingClose(null);
      } catch (err) {
        console.error("Failed to stop terminal:", err);
      }
    },
    [
      sessionId,
      environmentId,
      taskId,
      terminals,
      terminalTabValue,
      removeTerminal,
      setRightPanelActiveTab,
    ],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!pendingClose) return;
    await closeTerminal(pendingClose);
  }, [pendingClose, closeTerminal]);

  return { pendingClose, setPendingClose, handleConfirmClose, closeTerminal };
}

const MobileTerminalsList = memo(function MobileTerminalsList({
  sessionId,
  onClose,
}: {
  sessionId: string | null;
  onClose: () => void;
}) {
  const { terminals, terminalTabValue, addTerminal, removeTerminal, environmentId } =
    useMobileTerminalsContext();
  const setRightPanelActiveTab = useAppStore((s) => s.setRightPanelActiveTab);
  const taskId = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const { shells } = useUserShells(environmentId, taskId);
  const { pendingClose, setPendingClose, handleConfirmClose, closeTerminal } =
    useTerminalCloseHandler({
      sessionId,
      environmentId,
      taskId,
      terminals,
      terminalTabValue,
      removeTerminal,
      setRightPanelActiveTab,
    });

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

  const handleAskClose = useCallback(
    (terminal: Terminal) => {
      const needsConfirm = shouldConfirmTerminalClose(terminal.id, {
        type: terminal.type,
        kind: terminal.kind,
      });
      if (needsConfirm) {
        setPendingClose(terminal);
        return;
      }
      void closeTerminal(terminal);
    },
    [closeTerminal, setPendingClose],
  );

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
          onClick={() => addTerminal()}
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
        open={pendingClose !== null}
        terminalName={pendingClose?.label || "Terminal"}
        onOpenChange={(open) => {
          if (!open) setPendingClose(null);
        }}
        onConfirm={handleConfirmClose}
      />
    </div>
  );
});

function useActiveTerminalPillLabel(): { label: string; count: string | undefined } {
  const { terminals, terminalTabValue } = useMobileTerminalsContext();
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
  const { label, count } = useActiveTerminalPillLabel();
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
