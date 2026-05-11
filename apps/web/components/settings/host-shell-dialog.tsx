"use client";

import { startHostShell } from "@/lib/api";
import { PtyTerminalDialog } from "@/components/settings/pty-terminal-dialog";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Optional callback when the user clicks Done (e.g. trigger a rescan). */
  onClose?: () => void;
  /**
   * If set, this text is sent into the shell on connect - useful for
   * pre-filling a recovery command from an auth banner.
   */
  initialInput?: string;
};

/**
 * Opens a PTY-backed terminal running the user's shell on the kandev host.
 * Used for ad-hoc setup commands that don't fit the install/login flows.
 */
export function HostShellDialog({ open, onOpenChange, onClose, initialInput }: Props) {
  return (
    <PtyTerminalDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Host terminal"
      description="Runs a shell on the kandev host. Use it to install or configure agents manually."
      testIdPrefix="host-shell"
      startSession={startHostShell}
      onDone={onClose}
      initialInput={initialInput}
    />
  );
}
