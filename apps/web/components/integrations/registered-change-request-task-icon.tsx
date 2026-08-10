"use client";

import { useCallback, useEffect, useMemo, useSyncExternalStore } from "react";
import { IconGitPullRequest } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import {
  refreshReviewProvider,
  useReviewProviderUpdates,
} from "@/components/task/review-panel-provider";
import {
  ChangeRequestTaskStatusSummary,
  type ChangeRequestTaskStatusSummaryData,
  type ChangeRequestTaskSummaryRow,
} from "@/components/integrations/change-request-task-status-summary";
import { usePluginRegistry, type PluginReviewProviderRegistration } from "@/lib/plugins/registry";
import type { ReviewItemSummary, ReviewTaskStatus } from "@/lib/plugins/types";
import { useTranslation } from "react-i18next";
import { useChangeRequestTaskTooltipState } from "./use-change-request-task-tooltip-state";

type AssociationRefresh = NonNullable<PluginReviewProviderRegistration["refreshAssociations"]>;
type RefreshEntry = {
  controller: AbortController;
  consumers: number;
  settled: boolean;
  completedAt: number;
  done: Promise<void>;
};
const ASSOCIATION_REFRESH_INTERVAL_MS = 90_000;
const refreshes = new WeakMap<AssociationRefresh, Map<string, RefreshEntry>>();

function acquireAssociationRefresh(
  provider: PluginReviewProviderRegistration,
  workspaceId: string,
) {
  const refresh = provider.refreshAssociations!;
  const byWorkspace = refreshes.get(refresh) ?? new Map<string, RefreshEntry>();
  refreshes.set(refresh, byWorkspace);
  let entry = byWorkspace.get(workspaceId);
  if (entry?.settled && Date.now() - entry.completedAt >= ASSOCIATION_REFRESH_INTERVAL_MS) {
    byWorkspace.delete(workspaceId);
    entry = undefined;
  }
  if (!entry) {
    const controller = new AbortController();
    entry = { controller, consumers: 0, settled: false, completedAt: 0, done: Promise.resolve() };
    byWorkspace.set(workspaceId, entry);
    const current = entry;
    entry.done = refresh(workspaceId, controller.signal)
      .then(() => {
        current.settled = true;
        current.completedAt = Date.now();
      })
      .catch(() => {
        if (byWorkspace.get(workspaceId) === current) byWorkspace.delete(workspaceId);
      });
  }
  entry.consumers += 1;
  let released = false;
  return {
    release() {
      if (released) return;
      released = true;
      entry!.consumers -= 1;
      if (entry!.consumers === 0 && !entry!.settled && byWorkspace.get(workspaceId) === entry) {
        entry!.controller.abort();
        byWorkspace.delete(workspaceId);
      }
    },
  };
}

function useAssociationVersion(
  providers: PluginReviewProviderRegistration[],
  workspaceId: string | null,
) {
  const source = useMemo(() => {
    let version = 0;
    return {
      getSnapshot: () => version,
      subscribe: (listener: () => void) => {
        if (!workspaceId) return () => undefined;
        const unsubscribers = providers.map((provider) =>
          provider.subscribeAssociations!(workspaceId, () => {
            version += 1;
            listener();
          }),
        );
        return () => unsubscribers.forEach((unsubscribe) => unsubscribe());
      },
    };
  }, [providers, workspaceId]);
  return useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
}

function useRegisteredTaskAssociations(taskId: string) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const providers = useMemo(
    () =>
      registry
        .getReviewProviders()
        .filter(
          (provider) =>
            provider.getAssociationSnapshot &&
            provider.subscribeAssociations &&
            provider.refreshAssociations,
        ),
    [registry, registryVersion],
  );
  const associationVersion = useAssociationVersion(providers, workspaceId);
  useEffect(() => {
    if (!workspaceId) return;
    const leases = providers.map((provider) => acquireAssociationRefresh(provider, workspaceId));
    return () => leases.forEach((lease) => lease.release());
  }, [providers, workspaceId]);
  return useMemo(
    () =>
      workspaceId
        ? providers.flatMap((provider) =>
            provider.getAssociationSnapshot!(workspaceId)
              .filter((association) => association.taskId === taskId)
              .map((association) => ({ association, provider })),
          )
        : [],
    [associationVersion, providers, taskId, workspaceId],
  );
}

function taskStatusRows(status: ReviewTaskStatus): ChangeRequestTaskSummaryRow[] {
  const rows: ChangeRequestTaskSummaryRow[] = [];
  if (status.state === "merged") rows.push({ kind: "state", status: "merged", tone: "merged" });
  if (status.state === "closed") rows.push({ kind: "state", status: "closed", tone: "danger" });
  if (status.state === "draft") rows.push({ kind: "state", status: "draft", tone: "muted" });
  if (status.review?.state === "approved") {
    rows.push({ kind: "review", status: "approved", tone: "success" });
  } else if (status.review?.state === "changes_requested") {
    rows.push({ kind: "review", status: "changes_requested", tone: "danger" });
  } else if (status.review?.state === "pending") {
    rows.push({ kind: "review", status: "pending_review", tone: "info" });
  }
  if (status.pipelineState === "success") {
    rows.push({ kind: "ci", status: "passed", tone: "success" });
  } else if (status.pipelineState === "failure") {
    rows.push({ kind: "ci", status: "failed", tone: "danger" });
  } else if (status.pipelineState === "pending") {
    rows.push({ kind: "ci", status: "in_progress", tone: "warning" });
  }
  return rows;
}

function taskStatusSummary(review: ReviewItemSummary): ChangeRequestTaskStatusSummaryData | null {
  if (!review.taskStatus) return null;
  return {
    number: review.taskStatus.number,
    title: review.title,
    rows: taskStatusRows(review.taskStatus),
  };
}

export function RegisteredChangeRequestTaskIcon({ taskId }: { taskId: string }) {
  const { t } = useTranslation();
  const associations = useRegisteredTaskAssociations(taskId);
  const providers = useMemo(
    () => Array.from(new Set(associations.map(({ provider }) => provider))),
    [associations],
  );
  const reviewVersion = useReviewProviderUpdates(taskId, providers);
  const summaries = useMemo(() => {
    const keysByProvider = new Map<string, Set<string>>();
    associations.forEach(({ association, provider }) => {
      const keys = keysByProvider.get(provider.id) ?? new Set<string>();
      keys.add(association.reviewKey);
      keysByProvider.set(provider.id, keys);
    });
    return providers.flatMap((provider) => {
      const keys = keysByProvider.get(provider.id);
      if (!keys) return [];
      return provider
        .getSnapshot(taskId)
        .filter((review) => keys.has(review.reviewKey))
        .flatMap((review) => {
          const summary = taskStatusSummary(review);
          return summary ? [summary] : [];
        });
    });
  }, [associations, providers, reviewVersion, taskId]);
  const refreshDetails = useCallback(() => {
    providers.forEach((provider) => void refreshReviewProvider(provider, taskId));
  }, [providers, taskId]);
  const tooltip = useChangeRequestTaskTooltipState(refreshDetails);
  if (associations.length === 0) return null;
  const labels = Array.from(new Set(associations.map(({ provider }) => provider.label)));
  const providerLabel = labels.length === 1 ? labels[0] : t("integrations:registeredProvider");
  const count = associations.length;
  const label = t("integrations:linkedProviderPullRequests", { count, provider: providerLabel });
  return (
    <Tooltip open={tooltip.open}>
      <TooltipTrigger asChild>
        <span
          data-testid={`registered-change-request-task-icon-${taskId}`}
          role="img"
          tabIndex={0}
          aria-label={label}
          onPointerEnter={tooltip.onPointerEnter}
          onPointerLeave={tooltip.onPointerLeave}
          onFocus={tooltip.onFocus}
          onBlur={tooltip.onBlur}
          className="inline-flex shrink-0 items-center gap-0.5 text-muted-foreground"
        >
          <IconGitPullRequest aria-hidden="true" className="h-3.5 w-3.5" />
          {count > 1 ? (
            <span className="text-[9px] font-semibold leading-none">{count}</span>
          ) : null}
        </span>
      </TooltipTrigger>
      <TooltipContent
        sideOffset={6}
        onEscapeKeyDown={tooltip.onEscapeKeyDown}
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        {summaries.length > 0 ? <ChangeRequestTaskStatusSummary summaries={summaries} /> : label}
      </TooltipContent>
    </Tooltip>
  );
}
