import { useCallback, useEffect, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { lspClientManager, toLspLanguage, type LspStatus } from "@/lib/lsp/lsp-client-manager";

const DISABLED: LspStatus = { state: "disabled" };

export function useLsp(
  sessionId: string | null,
  monacoLanguage: string,
): {
  status: LspStatus;
  lspLanguage: string | null;
  toggle: () => void;
} {
  const lspAutoStartLanguages = useAppStore((s) => s.userSettings.lspAutoStartLanguages);
  const lspServerConfigs = useAppStore((s) => s.userSettings.lspServerConfigs);
  const lspLanguage = toLspLanguage(monacoLanguage);
  const shouldAutoStart = lspLanguage ? lspAutoStartLanguages.includes(lspLanguage) : false;

  const status = useSyncExternalStore(
    (cb) =>
      lspClientManager.onStatusChange((key) => {
        if (key === `${sessionId}:${lspLanguage}`) cb();
      }),
    () =>
      sessionId && lspLanguage ? lspClientManager.getStatus(sessionId, lspLanguage) : DISABLED,
  );

  // Auto-start: connect when file opens if language is in auto-start list
  useEffect(() => {
    if (!shouldAutoStart || !sessionId || !lspLanguage) return;
    const disconnect = lspClientManager.connect(sessionId, lspLanguage, lspServerConfigs);
    return disconnect;
  }, [shouldAutoStart, sessionId, lspLanguage, lspServerConfigs]);

  // Restore LSP state from localStorage: if the user previously enabled LSP
  // manually for this session+language, reconnect automatically on page load.
  useEffect(() => {
    if (!sessionId || !lspLanguage) return;
    if (shouldAutoStart) return;
    const current = lspClientManager.getStatus(sessionId, lspLanguage);
    if (current.state !== "disabled") return;

    if (lspClientManager.isEnabledInStorage(sessionId, lspLanguage)) {
      lspClientManager.connect(sessionId, lspLanguage, lspServerConfigs);
    }
  }, [sessionId, lspLanguage, shouldAutoStart, lspServerConfigs]);

  // Manual toggle
  const toggle = useCallback(() => {
    if (!sessionId || !lspLanguage) return;
    const current = lspClientManager.getStatus(sessionId, lspLanguage);
    if (
      current.state === "disabled" ||
      current.state === "error" ||
      current.state === "unavailable"
    ) {
      lspClientManager.connect(sessionId, lspLanguage, lspServerConfigs);
      lspClientManager.saveEnabledState(sessionId, lspLanguage);
    } else if (
      current.state === "ready" ||
      current.state === "connecting" ||
      current.state === "starting"
    ) {
      lspClientManager.stop(sessionId, lspLanguage);
      lspClientManager.clearEnabledState(sessionId, lspLanguage);
    }
  }, [sessionId, lspLanguage, lspServerConfigs]);

  return { status, lspLanguage, toggle };
}
