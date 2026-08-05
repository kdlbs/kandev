import { useCallback, useEffect, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { lspClientManager, toLspLanguage, type LspStatus } from "@/lib/lsp/lsp-client-manager";
import { EMPTY_LSP_PROGRESS, type LspProgressSnapshot } from "@/lib/lsp/lsp-progress";

const DISABLED: LspStatus = { state: "disabled" };
const startRequestGenerations = new Map<string, number>();
const manualStopOverrides = new Set<string>();

function lspKey(sessionId: string | null, lspLanguage: string | null): string | null {
  return sessionId && lspLanguage ? `${sessionId}:${lspLanguage}` : null;
}

function subscribeToLspKey(key: string | null, callback: () => void): () => void {
  if (!key) return () => {};
  return lspClientManager.onChange((changedKey) => {
    if (changedKey === key) callback();
  });
}

function requestLspStart(sessionId: string, lspLanguage: string): void {
  const key = `${sessionId}:${lspLanguage}`;
  manualStopOverrides.delete(key);
  startRequestGenerations.set(key, (startRequestGenerations.get(key) ?? 0) + 1);
  lspClientManager.saveEnabledState(sessionId, lspLanguage);
}

function requestLspStop(sessionId: string, lspLanguage: string): void {
  const key = `${sessionId}:${lspLanguage}`;
  manualStopOverrides.add(key);
  lspClientManager.stop(sessionId, lspLanguage);
  startRequestGenerations.delete(key);
  lspClientManager.clearEnabledState(sessionId, lspLanguage);
}

function toggleLsp(sessionId: string, lspLanguage: string): void {
  const current = lspClientManager.getStatus(sessionId, lspLanguage);
  if (
    current.state === "disabled" ||
    current.state === "error" ||
    current.state === "unavailable"
  ) {
    requestLspStart(sessionId, lspLanguage);
  } else if (
    current.state === "ready" ||
    current.state === "connecting" ||
    current.state === "installing" ||
    current.state === "starting"
  ) {
    requestLspStop(sessionId, lspLanguage);
  }
}

export function useLspStatus(sessionId: string | null, lspLanguage: string | null) {
  const key = lspKey(sessionId, lspLanguage);
  const status = useSyncExternalStore(
    (callback) => subscribeToLspKey(key, callback),
    () =>
      sessionId && lspLanguage ? lspClientManager.getStatus(sessionId, lspLanguage) : DISABLED,
  );
  const progress = useSyncExternalStore(
    (callback) => subscribeToLspKey(key, callback),
    () =>
      sessionId && lspLanguage
        ? lspClientManager.getProgress(sessionId, lspLanguage)
        : EMPTY_LSP_PROGRESS,
  );
  const toggle = useCallback(() => {
    if (sessionId && lspLanguage) toggleLsp(sessionId, lspLanguage);
  }, [sessionId, lspLanguage]);
  return { status, progress, toggle };
}

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
  const key = lspKey(sessionId, lspLanguage);
  const hasManualStopOverride = key ? manualStopOverrides.has(key) : false;
  const isManuallyEnabled = useSyncExternalStore(
    (callback) => subscribeToLspKey(key, callback),
    () =>
      sessionId && lspLanguage
        ? lspClientManager.isEnabledInStorage(sessionId, lspLanguage)
        : false,
  );
  const startRequestGeneration = useSyncExternalStore(
    (callback) => subscribeToLspKey(key, callback),
    () => (key ? (startRequestGenerations.get(key) ?? 0) : 0),
  );
  const { status, progress, toggle } = useLspStatus(sessionId, lspLanguage);

  // Each mounted matching editor owns one connection lease. An explicit Stop
  // suppresses global auto-start for this session/language until Start clears
  // the override; later settings/configuration renders must not reacquire it.
  useEffect(() => {
    const autoStartEnabled = shouldAutoStart && !hasManualStopOverride;
    if ((!autoStartEnabled && !isManuallyEnabled) || !sessionId || !lspLanguage) return;
    const disconnect = lspClientManager.connect(sessionId, lspLanguage, lspServerConfigs);
    return disconnect;
  }, [
    hasManualStopOverride,
    isManuallyEnabled,
    shouldAutoStart,
    sessionId,
    lspLanguage,
    lspServerConfigs,
    startRequestGeneration,
  ]);

  return { status, progress, lspLanguage, toggle };
}
