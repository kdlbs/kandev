"use client";

import { useMemo } from "react";
import { IconGitBranch, IconTerminal2 } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import type {
  LocalRepository,
  Repository,
  Branch,
  Executor,
  ExecutorProfile,
} from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { formatUserHomePath, truncateRepoPath } from "@/lib/utils";
import { getExecutorIcon } from "@/lib/executor-icons";
import { AgentLogo } from "@/components/agent-logo";

type OptionItem = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

export function useRepositoryOptions(
  repositories: Repository[],
  discoveredRepositories: LocalRepository[],
) {
  const repositoryOptions = useMemo(() => {
    const normalizeRepoPath = (path: string) => path.replace(/\\/g, "/").replace(/\/+$/g, "");
    const workspaceRepoPaths = new Set(
      repositories
        .map((repo: Repository) => repo.local_path)
        .filter(Boolean)
        .map((path: string) => normalizeRepoPath(path)),
    );
    const localRepoOptions = discoveredRepositories.filter(
      (repo: LocalRepository) => !workspaceRepoPaths.has(normalizeRepoPath(repo.path)),
    );
    return [
      ...repositories.map((repo: Repository) => ({
        value: repo.id,
        label: repo.name,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
            <span className="shrink-0">{repo.name}</span>
            <Badge
              variant="secondary"
              className="text-xs text-muted-foreground max-w-[140px] min-w-0 truncate ml-auto"
              title={formatUserHomePath(repo.local_path)}
            >
              {truncateRepoPath(repo.local_path, 24)}
            </Badge>
          </span>
        ),
      })),
      ...localRepoOptions.map((repo: LocalRepository) => ({
        value: repo.path,
        label: truncateRepoPath(repo.path, 24),
        renderLabel: () => (
          <span
            className="flex min-w-0 flex-1 items-center overflow-hidden"
            title={formatUserHomePath(repo.path)}
          >
            <span className="truncate">{truncateRepoPath(repo.path, 28)}</span>
          </span>
        ),
      })),
    ];
  }, [repositories, discoveredRepositories]);

  const headerRepositoryOptions = useMemo(() => {
    return repositoryOptions.map((opt) => ({
      ...opt,
      renderLabel: () => <span className="truncate">{opt.label}</span>,
    }));
  }, [repositoryOptions]);

  return { repositoryOptions, headerRepositoryOptions };
}

export function useBranchOptions(branchOptionsRaw: Branch[]) {
  return useMemo(() => {
    return branchOptionsRaw.map((branchObj: Branch) => {
      const displayName =
        branchObj.type === "remote" && branchObj.remote
          ? `${branchObj.remote}/${branchObj.name}`
          : branchObj.name;
      return {
        value: displayName,
        label: displayName,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
            <span className="flex min-w-0 items-center gap-1.5">
              <IconGitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate" title={displayName}>
                {displayName}
              </span>
            </span>
            <Badge variant="outline" className="text-xs">
              {branchObj.type === "local" ? "local" : branchObj.remote || "remote"}
            </Badge>
          </span>
        ),
      };
    });
  }, [branchOptionsRaw]);
}

export function useAgentProfileOptions(agentProfiles: AgentProfileOption[]): OptionItem[] {
  return useMemo(() => {
    return agentProfiles.map((profile: AgentProfileOption) => {
      const parts = profile.label.split(" \u2022 ");
      const agentLabel = parts[0] ?? profile.label;
      const profileLabel = parts[1] ?? "";
      const isPassthrough = profile.cli_passthrough === true;
      return {
        value: profile.id,
        label: profile.label,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
            <span className="flex shrink-0 items-center gap-1.5">
              <AgentLogo agentName={profile.agent_name} className="shrink-0" />
              <span>{agentLabel}</span>
            </span>
            <span className="flex shrink-0 items-center gap-1.5">
              {isPassthrough && (
                <IconTerminal2
                  className="size-3.5 text-muted-foreground"
                  title="Passthrough terminal"
                />
              )}
              {profileLabel ? (
                <ScrollOnOverflow className="rounded-full border border-border px-2 py-0.5 text-xs">
                  {profileLabel}
                </ScrollOnOverflow>
              ) : null}
            </span>
          </span>
        ),
      };
    });
  }, [agentProfiles]);
}

export function useExecutorOptions(executors: Executor[]): OptionItem[] {
  return useMemo(() => {
    return executors.map((executor: Executor) => {
      const Icon = getExecutorIcon(executor.type);
      return {
        value: executor.id,
        label: executor.name,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center gap-2">
            <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate">{executor.name}</span>
          </span>
        ),
      };
    });
  }, [executors]);
}

export function useExecutorHint(executors: Executor[], executorId: string): string | null {
  return useMemo(() => {
    const selectedExecutor = executors.find((e: Executor) => e.id === executorId);
    if (selectedExecutor?.type === "worktree")
      return "A git worktree will be created from the base branch.";
    if (selectedExecutor?.type === "local") return "The agent will run directly on the repository.";
    return null;
  }, [executors, executorId]);
}

export type ExecutorProfileOptionItem = OptionItem & {
  executorType?: string;
  executorName?: string;
};

export function useExecutorProfileOptions(
  allProfiles: ExecutorProfile[],
): ExecutorProfileOptionItem[] {
  return useMemo(() => {
    return allProfiles.map((profile) => {
      const Icon = getExecutorIcon(profile.executor_type ?? "local");
      return {
        value: profile.id,
        label: profile.name,
        executorType: profile.executor_type,
        executorName: profile.executor_name,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
            <span className="flex min-w-0 items-center gap-1.5">
              <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate">{profile.name}</span>
            </span>
            {profile.executor_name && (
              <Badge variant="outline" className="text-xs">
                {profile.executor_name}
              </Badge>
            )}
          </span>
        ),
      };
    });
  }, [allProfiles]);
}
