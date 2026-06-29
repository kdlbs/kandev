"use client";

import { useEffect, useRef } from "react";
import type { DockviewApi } from "dockview-react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { AppState } from "@/lib/state/store";

type DockviewPanel = NonNullable<ReturnType<DockviewApi["getPanel"]>>;
type ChangesCountState = Pick<AppState, "gitStatus" | "sessionCommits">;

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

export function selectChangesCountByEnvironment(state: ChangesCountState): Record<string, number> {
  const envKeys = new Set([
    ...Object.keys(state.gitStatus.byEnvironmentRepo),
    ...Object.keys(state.sessionCommits.byEnvironmentId),
  ]);
  const counts: Record<string, number> = {};

  for (const envKey of envKeys) {
    let count = state.sessionCommits.byEnvironmentId[envKey]?.length ?? 0;
    const repoStatuses = state.gitStatus.byEnvironmentRepo[envKey] ?? {};
    for (const status of Object.values(repoStatuses)) {
      count += Object.keys(status.files ?? {}).length;
    }
    counts[envKey] = count;
  }

  return counts;
}

export function markInactiveChangesCountIncreases(args: {
  countsByEnv: Record<string, number>;
  activeEnvKey: string | null;
  previousCounts: Record<string, number>;
  pendingEnvKeys: Set<string>;
}): void {
  const { countsByEnv, activeEnvKey, previousCounts, pendingEnvKeys } = args;
  for (const [envKey, count] of Object.entries(countsByEnv)) {
    const previous = previousCounts[envKey];
    previousCounts[envKey] = count;
    if (previous === undefined) continue;
    if (envKey !== activeEnvKey && count > previous) pendingEnvKeys.add(envKey);
  }
}

function useActiveEnvironmentKey(): string | null {
  return useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return state.environmentIdBySessionId[sessionId] ?? sessionId;
  });
}

/**
 * Queue a Changes-panel focus request when a background task/env receives new
 * changes, then consume it after the user switches into that environment.
 */
export function useChangesPanelAutoFocus() {
  const api = useDockviewStore((s) => s.api);
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);
  const activeEnvKey = useActiveEnvironmentKey();
  const countsByEnv = useAppStore(useShallow(selectChangesCountByEnvironment));
  const previousCountsRef = useRef<Record<string, number>>({});
  const pendingEnvKeysRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    markInactiveChangesCountIncreases({
      countsByEnv,
      activeEnvKey,
      previousCounts: previousCountsRef.current,
      pendingEnvKeys: pendingEnvKeysRef.current,
    });

    if (!activeEnvKey || isRestoringLayout) return;
    if (!pendingEnvKeysRef.current.has(activeEnvKey)) return;

    const result = activateChangesPanel(api);
    if (result !== "no-api") pendingEnvKeysRef.current.delete(activeEnvKey);
  }, [countsByEnv, activeEnvKey, api, isRestoringLayout]);
}
