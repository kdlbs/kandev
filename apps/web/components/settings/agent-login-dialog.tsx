"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { startAgentLogin } from "@/lib/api";
import { fetchDynamicModels } from "@/lib/api/domains/settings-api";
import { PtyTerminalDialog, type StartPtySession } from "@/components/settings/pty-terminal-dialog";

import { MiniMaxLoginRegion } from "./minimax-login-region";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentName: string;
  /** Optional human-readable description rendered above the terminal. */
  description?: string;
  /** Argv of the login command. Surfaced in the dialog so the user can see
   *  (and re-run) the actual command after Ctrl+C drops them into a shell. */
  command?: string[];
  variants?: Record<string, string[]>;
  /** Called when the user clicks Done. Used to trigger a capability rescan. */
  onLoginSuccess?: () => void;
  /** Refresh the catalog when the caller only rescans cached agent cards. */
  refreshModelsOnDone?: boolean;
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
  variants,
  onLoginSuccess,
  refreshModelsOnDone = false,
}: Props) {
  const { t } = useTranslation();
  const [selectedVariant, setSelectedVariant] = useState<string>();
  useEffect(() => {
    if (!open) setSelectedVariant(undefined);
  }, [open]);
  const startSession: StartPtySession = useCallback(
    (size, options) =>
      startAgentLogin(agentName, size, { ...options, commandVariant: selectedVariant }),
    [agentName, selectedVariant],
  );
  const handleLoginSuccess = useCallback(() => {
    if (!refreshModelsOnDone) {
      onLoginSuccess?.();
      return;
    }
    const finish = () => onLoginSuccess?.();
    void fetchDynamicModels(agentName, { refresh: true }).then(finish, finish);
  }, [agentName, onLoginSuccess, refreshModelsOnDone]);

  if (agentName === "minimax-acp" && variants?.cn && variants.global && !selectedVariant) {
    return (
      <MiniMaxLoginRegion open={open} onOpenChange={onOpenChange} onSelect={setSelectedVariant} />
    );
  }
  return (
    <PtyTerminalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("agents:signInToAgent", { name: agentName })}
      description={
        agentName === "minimax-acp"
          ? t("agents:minimaxLoginDescription", {
              dataDir: "~/.minimax",
            })
          : description
      }
      command={selectedVariant ? variants?.[selectedVariant] : command}
      presentation={agentName === "minimax-acp" ? "quick" : "standard"}
      testIdPrefix="agent-login"
      startSession={startSession}
      onDone={handleLoginSuccess}
    />
  );
}
