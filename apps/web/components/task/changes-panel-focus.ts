"use client";

import { useEffect, useRef } from "react";
import type { DockviewApi } from "dockview-react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { AppState } from "@/lib/state/store";
import type { FileInfo, GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

type DockviewPanel = NonNullable<ReturnType<DockviewApi["getPanel"]>>;
type ChangesMarkerState = Pick<AppState, "gitStatus" | "sessionCommits">;

export type ChangesMarker = {
  count: number;
  fingerprint: string;
};

export type ActivateChangesPanelResult =
  | "activated"
  | "blocked-agent-group"
  | "no-api"
  | "no-panel";

function groupContainsAgentSessionPanel(panel: DockviewPanel): boolean {
  return panel.group.panels.some((p) => p.id === "chat" || p.id.startsWith("session:"));
}

/** Activate the Changes panel unless it shares a group with agent sessions. */
export function activateChangesPanel(
  api: DockviewApi | null | undefined,
): ActivateChangesPanelResult {
  if (!api) return "no-api";

  const panel = api.getPanel("changes");
  if (!panel) return "no-panel";
  if (groupContainsAgentSessionPanel(panel)) return "blocked-agent-group";

  panel.api.setActive();
  return "activated";
}

export function autoActivateChangesPanel(): ActivateChangesPanelResult {
  return activateChangesPanel(useDockviewStore.getState().api);
}

function fileFingerprint(file: FileInfo): string {
  return [
    file.path,
    file.status,
    file.staged ? "1" : "0",
    file.additions ?? 0,
    file.deletions ?? 0,
    file.old_path ?? "",
    file.diff_skip_reason ?? "",
    file.repository_name ?? "",
  ].join(":");
}

function gitStatusFingerprint(repoName: string, status: GitStatusEntry): string {
  const fileKeys = Object.keys(status.files ?? {}).sort();
  const files = fileKeys.map((path) => fileFingerprint(status.files[path])).join(",");
  return [
    repoName,
    status.branch ?? "",
    status.remote_branch ?? "",
    status.ahead,
    status.behind,
    status.timestamp ?? "",
    status.repository_name ?? "",
    files,
  ].join("|");
}

export function selectChangesMarkerByEnvironment(
  state: ChangesMarkerState,
): Record<string, ChangesMarker> {
  const envKeys = new Set([
    ...Object.keys(state.gitStatus.byEnvironmentRepo),
    ...Object.keys(state.sessionCommits.byEnvironmentId),
  ]);
  const markers: Record<string, ChangesMarker> = {};

  for (const envKey of envKeys) {
    const commits = state.sessionCommits.byEnvironmentId[envKey] ?? [];
    let count = commits.length;
    const repoStatuses = state.gitStatus.byEnvironmentRepo[envKey] ?? {};
    const repoFingerprint = Object.entries(repoStatuses)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([repoName, status]) => {
        count += Object.keys(status.files ?? {}).length;
        return gitStatusFingerprint(repoName, status);
      })
      .join(";");
    const commitFingerprint = commits.map((commit) => commit.commit_sha).join(",");
    markers[envKey] = {
      count,
      fingerprint: `${repoFingerprint}#${commitFingerprint}`,
    };
  }

  return markers;
}

function markerToSignal(marker: ChangesMarker): string {
  // Zustand shallow comparison is stable for primitive values; count always
  // lives left of the first NUL and the full fingerprint lives to the right.
  return `${marker.count}\u0000${marker.fingerprint}`;
}

function signalToMarker(signal: string): ChangesMarker {
  const separatorIndex = signal.indexOf("\u0000");
  return {
    count: Number(signal.slice(0, separatorIndex)),
    fingerprint: signal.slice(separatorIndex + 1),
  };
}

function signalsToMarkers(signalsByEnv: Record<string, string>): Record<string, ChangesMarker> {
  return Object.fromEntries(
    Object.entries(signalsByEnv).map(([envKey, signal]) => [envKey, signalToMarker(signal)]),
  );
}

export function selectChangesSignalByEnvironment(
  state: ChangesMarkerState,
): Record<string, string> {
  const markers = selectChangesMarkerByEnvironment(state);
  return Object.fromEntries(
    Object.entries(markers).map(([envKey, marker]) => [envKey, markerToSignal(marker)]),
  );
}

function shouldQueueInactiveFocus(args: {
  envKey: string;
  activeEnvKey: string | null;
  previousActiveEnvKey: string | null;
  previous: ChangesMarker;
  next: ChangesMarker;
}): boolean {
  const { envKey, activeEnvKey, previousActiveEnvKey, previous, next } = args;
  const changed =
    next.count > previous.count || (next.count > 0 && next.fingerprint !== previous.fingerprint);
  if (!changed) return false;
  return (
    envKey !== activeEnvKey || (previousActiveEnvKey !== null && envKey !== previousActiveEnvKey)
  );
}

export function migrateEnvironmentKeys(args: {
  environmentIdBySessionId: Record<string, string>;
  previousMarkers: Record<string, ChangesMarker>;
  pendingEnvKeys: Set<string>;
}): void {
  const { environmentIdBySessionId, previousMarkers, pendingEnvKeys } = args;
  for (const [sessionId, envKey] of Object.entries(environmentIdBySessionId)) {
    if (sessionId === envKey) continue;
    if (pendingEnvKeys.delete(sessionId)) pendingEnvKeys.add(envKey);
    if (previousMarkers[sessionId] && !previousMarkers[envKey]) {
      previousMarkers[envKey] = previousMarkers[sessionId];
    }
    delete previousMarkers[sessionId];
  }
}

export function markInactiveChangesIncreases(args: {
  markersByEnv: Record<string, ChangesMarker>;
  activeEnvKey: string | null;
  previousActiveEnvKey: string | null;
  previousMarkers: Record<string, ChangesMarker>;
  pendingEnvKeys: Set<string>;
}): void {
  const { markersByEnv, activeEnvKey, previousActiveEnvKey, previousMarkers, pendingEnvKeys } =
    args;
  for (const [envKey, marker] of Object.entries(markersByEnv)) {
    const previous = previousMarkers[envKey];
    previousMarkers[envKey] = marker;
    if (previous === undefined) continue;
    if (
      shouldQueueInactiveFocus({
        envKey,
        activeEnvKey,
        previousActiveEnvKey,
        previous,
        next: marker,
      })
    ) {
      pendingEnvKeys.add(envKey);
    }
  }
}

export function shouldClearPendingChangesFocus(result: ActivateChangesPanelResult): boolean {
  return result === "activated" || result === "no-panel";
}

export function applyChangesPanelAutoFocusState(args: {
  signalsByEnv: Record<string, string>;
  activeEnvKey: string | null;
  previousActiveEnvKey: string | null;
  environmentIdBySessionId: Record<string, string>;
  previousMarkers: Record<string, ChangesMarker>;
  pendingEnvKeys: Set<string>;
  isRestoringLayout: boolean;
  activate: () => ActivateChangesPanelResult;
}): string | null {
  const {
    signalsByEnv,
    activeEnvKey,
    previousActiveEnvKey,
    environmentIdBySessionId,
    previousMarkers,
    pendingEnvKeys,
    isRestoringLayout,
    activate,
  } = args;

  migrateEnvironmentKeys({
    environmentIdBySessionId,
    previousMarkers,
    pendingEnvKeys,
  });

  markInactiveChangesIncreases({
    markersByEnv: signalsToMarkers(signalsByEnv),
    activeEnvKey,
    previousActiveEnvKey,
    previousMarkers,
    pendingEnvKeys,
  });

  if (activeEnvKey && !isRestoringLayout && pendingEnvKeys.has(activeEnvKey)) {
    const result = activate();
    if (shouldClearPendingChangesFocus(result)) pendingEnvKeys.delete(activeEnvKey);
  }

  return activeEnvKey;
}

export function useChangesPanelAutoFocus(activeEnvKey: string | null) {
  const api = useDockviewStore((s) => s.api);
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);
  const signalsByEnv = useAppStore(useShallow(selectChangesSignalByEnvironment));
  const environmentIdBySessionId = useAppStore((state) => state.environmentIdBySessionId);
  const previousMarkersRef = useRef<Record<string, ChangesMarker>>({});
  const pendingEnvKeysRef = useRef<Set<string>>(new Set());
  const previousActiveEnvKeyRef = useRef<string | null>(null);

  useEffect(() => {
    previousActiveEnvKeyRef.current = applyChangesPanelAutoFocusState({
      signalsByEnv,
      activeEnvKey,
      previousActiveEnvKey: previousActiveEnvKeyRef.current,
      environmentIdBySessionId,
      previousMarkers: previousMarkersRef.current,
      pendingEnvKeys: pendingEnvKeysRef.current,
      isRestoringLayout,
      activate: () => activateChangesPanel(api),
    });
  }, [signalsByEnv, activeEnvKey, api, isRestoringLayout, environmentIdBySessionId]);
}
