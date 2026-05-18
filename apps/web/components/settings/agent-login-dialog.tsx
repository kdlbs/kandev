"use client";

import { useCallback } from "react";
import { startAgentLogin } from "@/lib/api";
import { PtyTerminalDialog, type StartPtySession } from "@/components/settings/pty-terminal-dialog";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentName: string;
  /** Optional human-readable description rendered above the terminal. */
  description?: string;
  /** Argv of the login command. Surfaced in the dialog so the user can see
   *  (and re-run) the actual command after Ctrl+C drops them into a shell. */
  command?: string[];
  /** Called when the user clicks Done. Used to trigger a capability rescan. */
  onLoginSuccess?: () => void;
};

/**
 * Opens a PTY-backed terminal running an agent's login command on the kandev
 * host. Thin wrapper over <PtyTerminalDialog> - the only thing login-specific
 * is the start endpoint.
 */
export function AgentLoginDialog({
  open,
  onOpenChange,
  agentName,
  description,
  command,
  onLoginSuccess,
}: Props) {
  const startSession: StartPtySession = useCallback(
    (size, options) => startAgentLogin(agentName, size, options),
    [agentName],
  );

  return (
    <PtyTerminalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Sign in: ${agentName}`}
      description={description}
      command={command}
      testIdPrefix="agent-login"
      startSession={startSession}
      onDone={onLoginSuccess}
    />
  );
}
