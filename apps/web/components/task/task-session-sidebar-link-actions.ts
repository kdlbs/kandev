"use client";

import { createContext, createElement, useCallback, useContext, useMemo, useState } from "react";
import type { ReactNode } from "react";
import type { KanbanState } from "@/lib/state/slices";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type {
  PluginHostRepository,
  PluginIcon,
  PluginTaskActionContext,
} from "@/lib/plugins/types";
import type { PluginTaskActionRegistration } from "@/lib/plugins/registry";
import type { Repository } from "@/lib/types/http";
import { toPluginHostRepository } from "@/lib/plugins/host-repository";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePathname } from "@/lib/routing/client-router";
import type { ExternalLinkProvider } from "./task-external-link-dialog";
import { t } from "@/lib/i18n";

type StoreApi = {
  getState: () => {
    kanbanMulti: { snapshots: Record<string, { tasks: KanbanState["tasks"] }> };
    kanban: { tasks: KanbanState["tasks"] };
  };
};

const EMPTY_REPOSITORIES: Repository[] = [];

export type SidebarLinkTarget = {
  id: string;
  title: string;
  repositoryId?: string;
  issueUrl?: string;
  issueNumber?: number;
  repositories?: Array<{ id?: string; repository_id: string; position?: number }>;
};

export type SidebarExternalLinkTarget = {
  provider: ExternalLinkProvider;
  task: SidebarLinkTarget;
};

export type PluginLinkMenuAction = {
  id: string;
  label: string;
  icon?: PluginIcon;
  onSelect: () => void;
};

const PluginTaskLinkActionSurfaceContext = createContext<(() => void) | undefined>(undefined);

/** Lets a mobile drawer dismiss before a plugin opens its own host-native surface. */
export function PluginTaskLinkActionSurfaceProvider({
  beforePluginRun,
  children,
}: {
  beforePluginRun?: () => void;
  children: ReactNode;
}) {
  return createElement(
    PluginTaskLinkActionSurfaceContext.Provider,
    { value: beforePluginRun },
    children,
  );
}

export function runPluginTaskLinkAction(
  beforePluginRun: (() => void) | undefined,
  run: () => Promise<void>,
): void {
  beforePluginRun?.();
  void run().catch(() => {
    // Plugin action owns visible failure UI; keep host menu lifecycle safe.
  });
}

function readonlyClone(value: unknown, seen = new WeakMap<object, unknown>()): unknown {
  if (!value || typeof value !== "object") return value;
  const cached = seen.get(value);
  if (cached) return cached;
  const clone: unknown[] | Record<string, unknown> = Array.isArray(value) ? [] : {};
  seen.set(value, clone);
  for (const [key, child] of Object.entries(value)) {
    if (Array.isArray(clone)) clone[Number(key)] = readonlyClone(child, seen);
    else clone[key] = readonlyClone(child, seen);
  }
  return Object.freeze(clone);
}

export function immutablePluginTaskActionContext(
  context: PluginTaskActionContext,
): PluginTaskActionContext {
  return Object.freeze({
    ...context,
    repositories: readonlyClone(context.repositories) as readonly PluginHostRepository[],
  });
}

export function pluginTaskActionIsVisible(
  action: PluginTaskActionRegistration,
  context: PluginTaskActionContext,
): boolean {
  try {
    return action.visible?.(context) !== false;
  } catch {
    console.warn(`[plugins] task action "${action.pluginId}:${action.id}" visibility failed`);
    return false;
  }
}

/**
 * Registry actions receive only immutable, current task data. Menu callers
 * invoke this after closing their Radix surface, so plugins never run under an
 * open context menu or mobile bottom sheet.
 */
export function usePluginTaskLinkActions(
  context: PluginTaskActionContext | null,
): PluginLinkMenuAction[] {
  const registry = usePluginRegistry();
  const beforePluginRun = useContext(PluginTaskLinkActionSurfaceContext);
  return useMemo(() => {
    if (!context) return [];
    const actionContext = immutablePluginTaskActionContext(context);
    return registry
      .getTaskActions("link")
      .filter((action) => pluginTaskActionIsVisible(action, actionContext))
      .map((action) => ({
        id: `${action.pluginId}:${action.id}`,
        label: action.label,
        icon: action.icon,
        onSelect: () => {
          runPluginTaskLinkAction(beforePluginRun, () => action.run(actionContext));
        },
      }));
  }, [beforePluginRun, context, registry]);
}

export function pluginTaskRepositories(
  workspaceRepositories: readonly Repository[],
  repositoryLinks: readonly { repository_id: string; position?: number }[],
): PluginHostRepository[] {
  const byId = new Map(
    workspaceRepositories.map((repository) => [String(repository.id), repository]),
  );
  return [...repositoryLinks]
    .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
    .flatMap((link) => {
      const repository = byId.get(link.repository_id);
      return repository ? [toPluginHostRepository(repository)] : [];
    });
}

/** Builds the immutable action context shared by task-card and task-row menus. */
export function useTaskPluginLinkActions(
  taskId: string,
  repositoryLinks: readonly { repository_id: string; position?: number }[] = [],
) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceRepositories = useAppStore((state) =>
    workspaceId
      ? (state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
  const pathname = usePathname() ?? "";
  const { isMobile } = useResponsiveBreakpoint();
  const repositories = useMemo(
    () => pluginTaskRepositories(workspaceRepositories, repositoryLinks),
    [repositoryLinks, workspaceRepositories],
  );
  return usePluginTaskLinkActions(
    workspaceId
      ? {
          workspaceId,
          taskId,
          repositories,
          pathname,
          presentation: isMobile ? "mobile" : "desktop",
        }
      : null,
  );
}

export function useSidebarLinkActions(store: StoreApi) {
  const [linkingPullRequestTask, setLinkingPullRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingIssueTask, setLinkingIssueTask] = useState<SidebarLinkTarget | null>(null);
  const [linkingMergeRequestTask, setLinkingMergeRequestTask] = useState<SidebarLinkTarget | null>(
    null,
  );
  const [linkingExternalIssueTask, setLinkingExternalIssueTask] =
    useState<SidebarExternalLinkTarget | null>(null);

  const getLinkTarget = useCallback(
    (taskId: string, fallbackTitle?: string): SidebarLinkTarget => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      return {
        id: taskId,
        title: task?.title ?? fallbackTitle ?? t("task:thisTask"),
        repositoryId: task?.repositoryId,
        issueUrl: task?.issueUrl,
        issueNumber: task?.issueNumber,
        repositories: task?.repositories,
      };
    },
    [store],
  );

  const handleLinkPullRequestTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingPullRequestTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingIssueTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkMergeRequestTask = useCallback(
    (taskId: string, fallbackTitle?: string) => {
      setLinkingMergeRequestTask(getLinkTarget(taskId, fallbackTitle));
    },
    [getLinkTarget],
  );

  const handleLinkExternalIssueTask = useCallback(
    (provider: ExternalLinkProvider, taskId: string, fallbackTitle?: string) => {
      setLinkingExternalIssueTask({ provider, task: getLinkTarget(taskId, fallbackTitle) });
    },
    [getLinkTarget],
  );

  const handleLinkJiraTicketTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("jira", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  const handleLinkLinearIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("linear", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  const handleLinkSentryIssueTask = useCallback(
    (taskId: string, fallbackTitle?: string) =>
      handleLinkExternalIssueTask("sentry", taskId, fallbackTitle),
    [handleLinkExternalIssueTask],
  );

  return {
    linkingPullRequestTask,
    setLinkingPullRequestTask,
    handleLinkPullRequestTask,
    linkingIssueTask,
    setLinkingIssueTask,
    handleLinkIssueTask,
    linkingMergeRequestTask,
    setLinkingMergeRequestTask,
    handleLinkMergeRequestTask,
    linkingExternalIssueTask,
    setLinkingExternalIssueTask,
    handleLinkJiraTicketTask,
    handleLinkLinearIssueTask,
    handleLinkSentryIssueTask,
  };
}
