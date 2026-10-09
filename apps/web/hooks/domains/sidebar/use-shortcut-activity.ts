"use client";

import { useCallback, useMemo } from "react";
import { useAutomationSummaries } from "@/components/runs/use-automation-summaries";
import { useLiveRefresh } from "@/components/runs/use-live-refresh";
import { automationState, type AutomationActivityState } from "@/components/runs/automation-rows";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import type { Automation } from "@/lib/types/automation";

export type ShortcutActivityState = AutomationActivityState | "unknown";

export type ShortcutActivity = {
  state: ShortcutActivityState;
  loading: boolean;
  error: boolean;
};

export type ShortcutActivityReader = (automationId: string) => ShortcutActivity | undefined;

type UseShortcutActivityOptions = {
  workspaceId?: string;
  automations: Automation[];
  automationIds: string[];
  /** Catalog data can still be loading while the saved layout is available. */
  definitionsLoading?: boolean;
  definitionsError?: string | null;
  /** Hidden navigation surfaces do not keep a summary polling loop alive. */
  active?: boolean;
};

type UseShortcutActivityResult = {
  getActivity: (automationId: string) => ShortcutActivity;
  hasRunningActivity: (automationIds: string[]) => boolean;
  refresh: () => void;
};

const UNKNOWN_ACTIVITY: ShortcutActivity = { state: "unknown", loading: true, error: false };

export function useShortcutActivity({
  workspaceId,
  automations,
  automationIds,
  definitionsLoading = false,
  definitionsError = null,
  active = true,
}: UseShortcutActivityOptions): UseShortcutActivityResult {
  const hasPins = automationIds.length > 0;
  const summaryWorkspaceId = active && hasPins ? workspaceId : undefined;
  const summaries = useAutomationSummaries(summaryWorkspaceId);
  const refresh = useCallback(() => summaries.refresh(), [summaries.refresh]);

  useLiveRefresh(Boolean(summaryWorkspaceId), refresh);
  useForegroundRefresh(refresh, Boolean(summaryWorkspaceId), summaryWorkspaceId);

  const activities = useMemo(() => {
    const summariesByAutomationId = new Map(
      summaries.summaries.map((item) => [item.automation_id, item]),
    );
    const automationsById = new Map(automations.map((item) => [item.id, item]));
    const unavailable = definitionsError != null || summaries.error != null;
    const pending = definitionsLoading || summaries.loading;
    const next = new Map<string, ShortcutActivity>();

    for (const automationId of automationIds) {
      const automation = automationsById.get(automationId);
      if (pending || unavailable || !automation) {
        next.set(automationId, {
          state: "unknown",
          loading: pending,
          error: unavailable,
        });
        continue;
      }
      const item = summariesByAutomationId.get(automationId);
      next.set(automationId, {
        state: automationState(automation, item?.open_runs ?? 0),
        loading: false,
        error: false,
      });
    }
    return next;
  }, [
    automationIds,
    automations,
    definitionsError,
    definitionsLoading,
    summaries.error,
    summaries.loading,
    summaries.summaries,
  ]);

  const getActivity = useCallback(
    (automationId: string) => activities.get(automationId) ?? UNKNOWN_ACTIVITY,
    [activities],
  );
  const hasRunningActivity = useCallback(
    (ids: string[]) => ids.some((id) => activities.get(id)?.state === "running"),
    [activities],
  );

  return { getActivity, hasRunningActivity, refresh };
}
