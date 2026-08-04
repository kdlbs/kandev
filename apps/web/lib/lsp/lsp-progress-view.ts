import { t } from "@/lib/i18n";
import type { LspStatus } from "./lsp-json-rpc";
import type { LspProgressSnapshot } from "./lsp-progress";

export type LspLifecycleAction = {
  label: "Start" | "Stop" | "Retry" | "Stopping";
  enabled: boolean;
};

export const LSP_LONG_INITIALIZATION_MS = 60_000;

export type LspProgressView =
  | {
      kind: "active";
      title: string;
      message: string | null;
      percentage: number | null;
      elapsed: string;
      concurrentCount: number;
    }
  | {
      kind: "initializing";
      stage: "initialize_pending" | "long_running";
      title: string;
      description: string;
      guidance: string;
      elapsed: string;
    }
  | {
      kind: "completed";
      title: string;
      workTitle: string;
      message: string | null;
    }
  | { kind: "idle" | "waiting"; title: string; description: string };

export function formatLspElapsed(elapsedMs: number): string {
  const seconds = Math.max(0, Math.floor(elapsedMs / 1_000));
  if (seconds < 60) return `${seconds}s`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;

  const hours = Math.floor(minutes / 60);
  return `${hours}h ${String(minutes % 60).padStart(2, "0")}m`;
}

export function getLspConnectionLabel(status: LspStatus, progress?: LspProgressSnapshot): string {
  switch (status.state) {
    case "disabled":
      return t("lsp:off");
    case "connecting":
      return t("lsp:connecting");
    case "installing":
      return t("lsp:installingLanguageServer");
    case "starting":
      return progress?.initializingSince !== null && progress?.initializingSince !== undefined
        ? t("lsp:serverProcessStarted")
        : t("lsp:startingLanguageServer");
    case "ready":
      return t("lsp:connected");
    case "stopping":
      return t("lsp:stopping");
    case "unavailable":
      return t("lsp:unavailable");
    case "error":
      return t("lsp:error");
  }
}

export function getLspCompactSummary(
  status: LspStatus,
  snapshot: LspProgressSnapshot,
  now: number,
): string {
  const active = snapshot.active[0];
  if (active) {
    const suffix =
      active.percentage === null
        ? formatLspElapsed(now - active.startedAt)
        : `${active.percentage}%`;
    return `${active.title} · ${suffix}`;
  }
  if (snapshot.initializingSince !== null) {
    return `${getLspConnectionLabel(status, snapshot)} · ${formatLspElapsed(
      now - snapshot.initializingSince,
    )}`;
  }
  return getLspConnectionLabel(status, snapshot);
}

export function getLspLifecycleAction(status: LspStatus): LspLifecycleAction {
  switch (status.state) {
    case "disabled":
      return { label: "Start", enabled: true };
    case "unavailable":
    case "error":
      return { label: "Retry", enabled: true };
    case "stopping":
      return { label: "Stopping", enabled: false };
    default:
      return { label: "Stop", enabled: true };
  }
}

function getInitializationGuidance(longRunning: boolean, language?: string): string {
  if (!longRunning) return t("lsp:crossFileFeaturesAfterInitialization");
  if (language?.toLowerCase() === "kotlin") {
    return t("lsp:kotlinInitializationGuidance");
  }
  return t("lsp:crossFileFeaturesUnavailable");
}

export function getLspProgressView(
  status: LspStatus,
  snapshot: LspProgressSnapshot,
  now: number,
  language?: string,
): LspProgressView {
  const active = snapshot.active[0];
  if (active) {
    return {
      kind: "active",
      title: active.title,
      message: active.message,
      percentage: active.percentage,
      elapsed: formatLspElapsed(now - active.startedAt),
      concurrentCount: snapshot.active.length,
    };
  }

  if (snapshot.initializingSince !== null) {
    const elapsedMs = Math.max(0, now - snapshot.initializingSince);
    const longRunning = elapsedMs >= LSP_LONG_INITIALIZATION_MS;
    return {
      kind: "initializing",
      stage: longRunning ? "long_running" : "initialize_pending",
      title: longRunning ? t("lsp:initializationLongRunning") : t("lsp:serverProcessStarted"),
      description: longRunning
        ? t("lsp:serverStillInitializing")
        : t("lsp:waitingForInitializeResponse"),
      guidance: getInitializationGuidance(longRunning, language),
      elapsed: formatLspElapsed(elapsedMs),
    };
  }

  if (snapshot.completed) {
    return {
      kind: "completed",
      title: t("lsp:serverWorkFinished"),
      workTitle: snapshot.completed.title,
      message: snapshot.completed.message,
    };
  }

  if (status.state === "ready") {
    return {
      kind: "idle",
      title: t("lsp:noBackgroundWorkReported"),
      description: t("lsp:noBackgroundWorkReportedDescription"),
    };
  }

  if (status.state === "disabled") {
    return {
      kind: "waiting",
      title: t("lsp:languageServerIsOff"),
      description: t("lsp:startServerForProgress"),
    };
  }

  if (status.state === "error" || status.state === "unavailable") {
    return {
      kind: "waiting",
      title: t("lsp:progressUnavailable"),
      description: t("lsp:progressUnavailableDescription"),
    };
  }

  return {
    kind: "waiting",
    title: t("lsp:waitingForLanguageServer"),
    description:
      status.state === "stopping"
        ? t("lsp:progressWillClear")
        : t("lsp:progressAppearsDuringInitialization"),
  };
}
