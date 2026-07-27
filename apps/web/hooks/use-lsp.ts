import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { lspClientManager, toLspLanguage, type LspStatus } from "@/lib/lsp/lsp-client-manager";
import { EMPTY_LSP_PROGRESS, type LspProgressSnapshot } from "@/lib/lsp/lsp-progress";

const DISABLED: LspStatus = { state: "disabled" };

export function useLsp(
  sessionId: string | null,
  monacoLanguage: string,
): {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string | null;
  toggle: () => void;
} {
  const lspAutoStartLanguages = useAppStore((s) => s.userSettings.lspAutoStartLanguages);
  const lspServerConfigs = useAppStore((s) => s.userSettings.lspServerConfigs);
  const lspLanguage = toLspLanguage(monacoLanguage);
  const shouldAutoStart = lspLanguage ? lspAutoStartLanguages.includes(lspLanguage) : false;
  const [retryGeneration, setRetryGeneration] = useState(0);
  const isManuallyEnabled = useSyncExternalStore(
    (cb) =>
      lspClientManager.onChange((key) => {
        if (key === `${sessionId}:${lspLanguage}`) cb();
      }),
    () =>
      sessionId && lspLanguage
        ? lspClientManager.isEnabledInStorage(sessionId, lspLanguage)
        : false,
  );

  const status = useSyncExternalStore(
    (cb) =>
      lspClientManager.onChange((key) => {
        if (key === `${sessionId}:${lspLanguage}`) cb();
      }),
    () =>
      sessionId && lspLanguage ? lspClientManager.getStatus(sessionId, lspLanguage) : DISABLED,
  );

  const progress = useSyncExternalStore(
    (cb) =>
      lspClientManager.onChange((key) => {
        if (key === `${sessionId}:${lspLanguage}`) cb();
      }),
    () =>
      sessionId && lspLanguage
        ? lspClientManager.getProgress(sessionId, lspLanguage)
        : EMPTY_LSP_PROGRESS,
  );

  // Each mounted matching editor owns one connection lease. Manual policy and
  // auto-start only decide whether the editor should acquire that lease.
  useEffect(() => {
    if ((!shouldAutoStart && !isManuallyEnabled) || !sessionId || !lspLanguage) return;
    const disconnect = lspClientManager.connect(sessionId, lspLanguage, lspServerConfigs);
    return disconnect;
  }, [
    isManuallyEnabled,
    shouldAutoStart,
    sessionId,
    lspLanguage,
    lspServerConfigs,
    retryGeneration,
  ]);

  // Manual toggle
  const toggle = useCallback(() => {
    if (!sessionId || !lspLanguage) return;
    const current = lspClientManager.getStatus(sessionId, lspLanguage);
    if (
      current.state === "disabled" ||
      current.state === "error" ||
      current.state === "unavailable"
    ) {
      lspClientManager.saveEnabledState(sessionId, lspLanguage);
      setRetryGeneration((generation) => generation + 1);
    } else if (
      current.state === "ready" ||
      current.state === "connecting" ||
      current.state === "installing" ||
      current.state === "starting"
    ) {
      lspClientManager.stop(sessionId, lspLanguage);
      lspClientManager.clearEnabledState(sessionId, lspLanguage);
    }
  }, [sessionId, lspLanguage]);

  return { status, progress, lspLanguage, toggle };
}
