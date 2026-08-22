"use client";

import { memo, useCallback, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useUserShells } from "@/hooks/domains/session/use-user-shells";
import { releaseAutoCreatedEnvironment } from "@/hooks/domains/session/use-mobile-terminals";
import { MobilePillButton } from "./mobile-pill-button";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import { MobileTerminalRow } from "./mobile-terminal-row";
import { useMobileTerminalsContext } from "./mobile-terminals-context";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { useTranslation } from "react-i18next";

type CloseHandlerArgs = {
  environmentId: string | null;
  terminals: Terminal[];
  destroyTerminal: (id: string) => Promise<boolean>;
  addTerminal: () => void;
};

function useTerminalCloseHandler({
  environmentId,
  terminals,
  destroyTerminal,
  addTerminal,
}: CloseHandlerArgs) {
  const [pendingClose, setPendingClose] = useState<Terminal | null>(null);

  const closeTerminal = useCallback(
    async (terminal: Terminal) => {
      const succeeded = await destroyTerminal(terminal.id);
      if (succeeded && environmentId && terminals.length <= 1) {
        releaseAutoCreatedEnvironment(environmentId);
        addTerminal();
      }
      return succeeded;
    },
    [destroyTerminal, environmentId, terminals.length, addTerminal],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!pendingClose) return;
    const terminal = pendingClose;
    setPendingClose(null);
    await closeTerminal(terminal);
  }, [pendingClose, closeTerminal]);

  return { pendingClose, setPendingClose, handleConfirmClose };
}

function MobileTerminalsHeader({ count, onAdd }: { count: number; onAdd: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between px-1">
      <span className="text-xs font-medium text-muted-foreground">
        {t("task:terminalCount", { count })}
      </span>
      <Button
        size="sm"
        variant="outline"
        className="h-7 gap-1 cursor-pointer"
        onClick={onAdd}
        data-testid="mobile-add-terminal"
      >
        <IconPlus className="h-4 w-4" />
        {t("task:newTerminal")}
      </Button>
    </div>
  );
}

const MobileTerminalsList = memo(function MobileTerminalsList({
  sessionId,
  onClose,
}: {
  sessionId: string | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { terminals, terminalTabValue, addTerminal, destroyTerminal, environmentId } =
    useMobileTerminalsContext();
  const setRightPanelActiveTab = useAppStore((s) => s.setRightPanelActiveTab);
  const taskId = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const { shells } = useUserShells(environmentId, taskId);
  const { pendingClose, setPendingClose, handleConfirmClose } = useTerminalCloseHandler({
    environmentId,
    terminals,
    destroyTerminal,
    addTerminal,
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
      setPendingClose(terminal);
    },
    [setPendingClose],
  );

  if (!sessionId) {
    return (
      <div className="text-xs text-muted-foreground px-2 py-6 text-center">
        {t("task:noActiveSession")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2 px-1">
      <MobileTerminalsHeader count={terminals.length} onAdd={addTerminal} />
      <div className="flex flex-col gap-0.5">
        {terminals.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            {t("task:noTerminalsYet")}
          </div>
        )}
        {terminals.map((t) => (
          <MobileTerminalRow
            key={t.id}
            terminal={t}
            isActive={t.id === terminalTabValue}
            isRunning={isShellRunning(t.id)}
            isConfirming={pendingClose?.id === t.id}
            onSelect={handleSelect}
            onAskClose={handleAskClose}
            onCancelClose={() => setPendingClose(null)}
            onClose={() => setPendingClose(null)}
            onConfirmClose={handleConfirmClose}
          />
        ))}
      </div>
    </div>
  );
});

function useActiveTerminalPillLabel(): { label: string; count: string | undefined } {
  const { t } = useTranslation();
  const { terminals, terminalTabValue } = useMobileTerminalsContext();
  const activeIdx = terminals.findIndex((t) => t.id === terminalTabValue);
  const idx = activeIdx >= 0 ? activeIdx : 0;
  const active = terminals[idx];
  const total = terminals.length;
  let count: string | undefined;
  if (total > 1) count = `${idx + 1}/${total}`;
  return { label: active?.label ?? t("task:terminal"), count };
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
  const { t } = useTranslation();
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
        ariaLabel={t("task:activeTerminalTapToSwitch", { label })}
      />
      <MobilePickerSheet open={open} onOpenChange={setOpen} title={t("task:terminals")}>
        <MobileTerminalsList sessionId={sessionId} onClose={() => setOpen(false)} />
      </MobilePickerSheet>
    </>
  );
});
