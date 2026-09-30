"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { startAgentLogin } from "@/lib/api";
import { fetchDynamicModels } from "@/lib/api/domains/settings-api";
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
 * host. MiniMax refreshes the native catalog before settings reread cached
 * agent cards.
 */
export function AgentLoginDialog({
  open,
  onOpenChange,
  agentName,
  description,
  command,
  onLoginSuccess,
}: Props) {
  const { t } = useTranslation();
  const startSession: StartPtySession = useCallback(
    (size, options) => startAgentLogin(agentName, size, options),
    [agentName],
  );
  const handleLoginSuccess = useCallback(() => {
    if (agentName !== "minimax-acp") {
      onLoginSuccess?.();
      return;
    }
    const finish = () => onLoginSuccess?.();
    void fetchDynamicModels(agentName, { refresh: true }).then(finish, finish);
  }, [agentName, onLoginSuccess]);

  return (
    <PtyTerminalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("agents:signInToAgent", { name: agentName })}
      description={agentName === "minimax-acp" ? t("agents:minimaxLoginDescription") : description}
      command={command}
      presentation={agentName === "minimax-acp" ? "quick" : "standard"}
      testIdPrefix="agent-login"
      startSession={startSession}
      onDone={handleLoginSuccess}
    />
  );
}
