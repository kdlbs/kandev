import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { prTaskKey } from "@/components/github/pr-utils";
import { usePluginRegistry, type PluginReviewProviderRegistration } from "@/lib/plugins/registry";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskPR } from "@/lib/types/github";
import { t } from "@/lib/i18n";

/** Provider IDs are registry-owned strings; GitHub and GitLab are built-in adapters. */
export type ReviewProvider = string;

const EMPTY_GITHUB_REVIEWS: TaskPR[] = [];
const EMPTY_GITLAB_REVIEWS: TaskMR[] = [];

function githubReviewItem(pr: TaskPR): ReviewItemSummary {
  return {
    providerId: "github",
    reviewKey: prTaskKey(pr),
    title: pr.pr_title || t("github:pullRequestNumbered", { number: pr.pr_number }),
    url: pr.pr_url,
    connectionScope: reviewConnectionScope(pr.pr_url, "github"),
    repositoryId: pr.repository_id || `${pr.owner}/${pr.repo}`,
    changeRequestNumber: pr.pr_number,
    state: pr.state,
    ...(pr.checks_state ? { statusBadge: { label: pr.checks_state } } : {}),
  };
}

function gitLabReviewItem(mr: TaskMR): ReviewItemSummary {
  return {
    providerId: "gitlab",
    reviewKey: mrTaskKey(mr),
    title: mr.mr_title || `Merge Request !${mr.mr_iid}`,
    url: mr.mr_url,
    connectionScope: mr.host || reviewConnectionScope(mr.mr_url, "gitlab"),
    repositoryId: mr.repository_id || mr.project_path,
    changeRequestNumber: mr.mr_iid,
    state: mr.state,
    ...(mr.pipeline_state ? { statusBadge: { label: mr.pipeline_state } } : {}),
  };
}

function reviewConnectionScope(url: string, fallback: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return fallback;
  }
}

export function useReviewProviderUpdates(
  taskId: string | null,
  providers: PluginReviewProviderRegistration[],
): number {
  const source = useMemo(() => {
    let version = 0;
    return {
      getSnapshot: () => version,
      subscribe: (listener: () => void) => {
        if (!taskId) return () => {};
        return combineUnsubscribers(
          providers.map((provider) =>
            provider.subscribe(taskId, () => {
              version += 1;
              listener();
            }),
          ),
        );
      },
    };
  }, [taskId, providers]);
  // Version snapshots stay stable even when a provider allocates an array on
  // every getSnapshot call, which is permitted by the plugin boundary.
  return useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
}

type ProviderRefreshEntry = {
  controller: AbortController;
  consumers: number;
  settled: boolean;
  done: Promise<void>;
};

const providerRefreshes = new WeakMap<
  PluginReviewProviderRegistration["refresh"],
  Map<string, ProviderRefreshEntry>
>();

function acquireProviderRefresh(
  provider: PluginReviewProviderRegistration,
  taskId: string,
): { done: Promise<void>; release: () => void } {
  const refresh = provider.refresh;
  const byTask = providerRefreshes.get(refresh) ?? new Map<string, ProviderRefreshEntry>();
  providerRefreshes.set(refresh, byTask);
  let entry = byTask.get(taskId);
  if (!entry) {
    const controller = new AbortController();
    const nextEntry: ProviderRefreshEntry = {
      controller,
      consumers: 0,
      settled: false,
      done: Promise.resolve(),
    };
    entry = nextEntry;
    byTask.set(taskId, entry);
    entry.done = provider
      .refresh(taskId, controller.signal)
      .catch(() => undefined)
      .finally(() => {
        nextEntry.settled = true;
        if (byTask.get(taskId) === nextEntry) byTask.delete(taskId);
        if (byTask.size === 0) providerRefreshes.delete(refresh);
      });
  }
  entry.consumers += 1;
  let released = false;
  const release = () => {
    if (released) return;
    released = true;
    entry!.consumers -= 1;
    if (entry!.consumers === 0 && !entry!.settled) {
      entry!.controller.abort();
      if (byTask.get(taskId) === entry) byTask.delete(taskId);
      if (byTask.size === 0) providerRefreshes.delete(refresh);
    }
  };
  return { done: entry.done, release };
}

/** Shares one provider refresh across task chrome, review selectors, and manual refreshes. */
export async function refreshReviewProvider(
  provider: PluginReviewProviderRegistration,
  taskId: string,
): Promise<void> {
  const lease = acquireProviderRefresh(provider, taskId);
  try {
    await lease.done;
  } finally {
    lease.release();
  }
}

/**
 * Hydrates registered review summaries while a host surface needs them. The
 * shared lease keeps duplicate sidebar, topbar, and panel consumers on one
 * provider request and aborts it when the final consumer disappears.
 */
export function useReviewProviderRefreshes(
  taskId: string | null,
  providers: PluginReviewProviderRegistration[],
): void {
  useEffect(() => {
    if (!taskId || providers.length === 0) return;
    const leases = providers.map((provider) => acquireProviderRefresh(provider, taskId));
    return () => leases.forEach((lease) => lease.release());
  }, [providers, taskId]);
}

function combineUnsubscribers(unsubscribers: Array<() => void>): () => void {
  return () => unsubscribers.forEach((unsubscribe) => unsubscribe());
}

/**
 * Single normalized selector for built-in and registered reviews. Plugin
 * providers refresh through their external-store subscription; their panels
 * remain revocable because registry changes re-render this hook.
 */
export function useNormalizedTaskReviewsState(taskId: string | null): {
  reviews: readonly ReviewItemSummary[];
  loading: boolean;
} {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const prs = useAppStore((state) =>
    taskId ? (state.taskPRs.byTaskId[taskId] ?? EMPTY_GITHUB_REVIEWS) : EMPTY_GITHUB_REVIEWS,
  );
  const mrs = useAppStore((state) =>
    taskId && workspaceId
      ? (state.taskMRs.byWorkspaceId[workspaceId]?.[taskId] ?? EMPTY_GITLAB_REVIEWS)
      : EMPTY_GITLAB_REVIEWS,
  );
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const providers = useMemo(() => registry.getReviewProviders(), [registry, registryVersion]);
  const providerVersion = useReviewProviderUpdates(taskId, providers);
  const refreshScope = taskId && providers.length ? `${taskId}:${registryVersion}` : "";
  const [settledRefreshScope, setSettledRefreshScope] = useState("");

  useEffect(() => {
    if (!taskId || !refreshScope) return;
    let active = true;
    const leases = providers.map((provider) => acquireProviderRefresh(provider, taskId));
    void Promise.all(leases.map((lease) => lease.done)).then(() => {
      if (active) setSettledRefreshScope(refreshScope);
    });
    return () => {
      active = false;
      leases.forEach((lease) => lease.release());
    };
  }, [providers, refreshScope, taskId]);

  const reviews = useMemo(
    () => [
      ...prs.map(githubReviewItem),
      ...mrs.map(gitLabReviewItem),
      ...(taskId ? providers.flatMap((provider) => provider.getSnapshot(taskId)) : []),
    ],
    [mrs, prs, providers, providerVersion, taskId],
  );
  return { reviews, loading: Boolean(refreshScope && settledRefreshScope !== refreshScope) };
}

export function useNormalizedTaskReviews(taskId: string | null): readonly ReviewItemSummary[] {
  return useNormalizedTaskReviewsState(taskId).reviews;
}

export function resolveReviewPanelProvider(
  params: {
    providerId?: unknown;
    provider?: unknown;
    reviewKey?: unknown;
    prKey?: unknown;
    mrKey?: unknown;
  },
  hasGitHubPR: boolean,
  hasGitLabMR: boolean,
): ReviewProvider | null {
  if (typeof params.providerId === "string" && params.providerId) return params.providerId;
  if (params.provider === "gitlab" || typeof params.mrKey === "string") return "gitlab";
  if (params.provider === "github" || typeof params.prKey === "string") return "github";
  if (hasGitHubPR) return "github";
  if (hasGitLabMR) return "gitlab";
  return null;
}

/** Normalize new provider-neutral params while retaining every saved-layout alias. */
export function resolveReviewKey(params: {
  reviewKey?: unknown;
  prKey?: unknown;
  mrKey?: unknown;
}): string | undefined {
  if (typeof params.reviewKey === "string") return params.reviewKey;
  if (typeof params.prKey === "string") return params.prKey;
  if (typeof params.mrKey === "string") return params.mrKey;
  return undefined;
}
