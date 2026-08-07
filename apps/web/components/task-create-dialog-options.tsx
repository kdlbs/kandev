"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { IconAlertTriangle, IconGitBranch, IconTerminal2 } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import type {
  LocalRepository,
  Repository,
  Branch,
  Executor,
  ExecutorProfile,
} from "@/lib/types/http";
import type { AvailableAgent } from "@/lib/types/http-agents";
import type { AgentProfileOption } from "@/lib/state/slices";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import { formatUserHomePath, truncateRepoPath } from "@/lib/utils";
import { getExecutorIcon } from "@/lib/executor-icons";
import { AgentLogo } from "@/components/agent-logo";
import { getCapabilityWarning } from "@/lib/capability-warning";
import { buildBranchKeywords } from "./task-create-dialog-pill";

type OptionItem = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
  disabled?: boolean;
  disabledReason?: string;
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
            {repo.local_path ? (
              <Badge
                variant="secondary"
                className="text-xs text-muted-foreground max-w-[140px] min-w-0 truncate ml-auto"
                title={formatUserHomePath(repo.local_path)}
              >
                {truncateRepoPath(repo.local_path, 24)}
              </Badge>
            ) : null}
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
      // Keywords give the scorer extra surfaces to match against: the leaf
      // branch name, every path segment, and (for remotes) the remote name.
      const keywords = buildBranchKeywords(branchObj.name, branchObj.remote);
      return {
        value: displayName,
        label: displayName,
        keywords,
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

// advertisedModelIDs returns the currently advertised model IDs for an agent
// from the host-utility probe cache (empty when the probe has not landed).
function advertisedModelIDs(availableAgents: AvailableAgent[], agentName: string): string[] {
  const agent = availableAgents.find((a) => a.name === agentName);
  return agent?.model_config?.available_models?.map((m) => m.id) ?? [];
}

export function useAgentProfileOptions(agentProfiles: AgentProfileOption[]): OptionItem[] {
  const { t } = useTranslation();
  const { items: availableAgents } = useAvailableAgents();
  return useMemo(() => {
    // Disabled profiles stay in the store (existing sessions keep their
    // labels) but are never offered as a choice for new work.
    const selectable = agentProfiles.filter(isSelectableAgentProfile);
    return selectable.map((profile: AgentProfileOption) => {
      const parts = profile.label.split(" \u2022 ");
      const agentLabel = parts[0] ?? profile.label;
      const profileLabel = parts[1] ?? "";
      const isPassthrough = profile.cli_passthrough === true;
      const warning = getCapabilityWarning(profile.capability_status, profile.capability_error);
      // No-silent-model-fallback: a profile whose start model is no longer
      // advertised ("gone") is blocked from selection unless it opted into
      // an explicit fallback model (shown as a warning) or the legacy
      // auto-fallback toggle.
      //
      // The advertised list is the host-utility probe cache. While it has
      // not loaded yet (empty list) gone-ness is unknown and deliberately
      // NOT assumed: nothing is blocked and no warning is shown. The backend
      // rejects the launch if the model is genuinely unavailable, so this
      // startup window is cosmetic. A fallback model that is itself gone
      // does not make the profile selectable — it would promise a switch
      // SetModel cannot apply, so the profile is blocked like strict mode.
      const advertised = advertisedModelIDs(availableAgents, profile.agent_name);
      const startModelGone = Boolean(
        profile.model && advertised.length > 0 && !advertised.includes(profile.model),
      );
      const fallbackGone = Boolean(
        profile.fallback_model &&
        advertised.length > 0 &&
        !advertised.includes(profile.fallback_model),
      );
      const autoFallbackOn = profile.auto_fallback === true;
      const fallbackUsable = Boolean(profile.fallback_model) && !fallbackGone;
      const blocked = startModelGone && !autoFallbackOn && !fallbackUsable;
      const fallbackWarning = startModelGone && !autoFallbackOn && fallbackUsable;
      const disabledReason = blocked
        ? t("settings:profileStartModelUnavailable", { model: profile.model })
        : undefined;
      const fallbackNote = fallbackWarning
        ? t("settings:profileFallbackWillBeUsed", {
            model: profile.model,
            fallback: profile.fallback_model,
          })
        : undefined;
      return {
        value: profile.id,
        label: profile.label,
        disabled: blocked || undefined,
        disabledReason,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
            <span className="flex shrink-0 items-center gap-1.5">
              <AgentLogo agentName={profile.agent_name} className="shrink-0" />
              <span>{agentLabel}</span>
              {warning && (
                <warning.Icon className={`size-3.5 ${warning.color}`} title={warning.title} />
              )}
              {fallbackNote && (
                <IconAlertTriangle
                  className="size-3.5 shrink-0 text-amber-500"
                  title={fallbackNote}
                />
              )}
            </span>
            <span className="flex shrink-0 items-center gap-1.5">
              {isPassthrough && (
                <IconTerminal2
                  className="size-3.5 text-muted-foreground"
                  title={t("task:cliModeYourPromptWillBe")}
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
  }, [agentProfiles, availableAgents, t]);
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

export function useIsLocalExecutor(executors: Executor[], executorId: string): boolean {
  return useMemo(() => {
    const selected = executors.find((e: Executor) => e.id === executorId);
    return selected?.type === "local" || selected?.type === "local_pc";
  }, [executors, executorId]);
}

export function computeExecutorHint(
  executors: Executor[],
  executorId: string,
  repoCount: number,
): string | null {
  const selectedExecutor = executors.find((e: Executor) => e.id === executorId);
  if (selectedExecutor?.type === "worktree") {
    if (repoCount > 1) {
      return t("task:executorHintWorktreeMulti");
    }
    return t("task:executorHintWorktreeSingle");
  }
  if (selectedExecutor?.type === "local_docker" || selectedExecutor?.type === "remote_docker") {
    return t("task:executorHintDocker");
  }
  if (selectedExecutor?.type === "local" || selectedExecutor?.type === "local_pc")
    return t("task:executorHintLocal");
  return null;
}

export function useExecutorHint(
  executors: Executor[],
  executorId: string,
  repoCount: number,
): string | null {
  return useMemo(
    () => computeExecutorHint(executors, executorId, repoCount),
    [executors, executorId, repoCount],
  );
}

export type ExecutorProfileOptionItem = OptionItem & {
  executorType?: string;
  executorName?: string;
};

export type ExecutorProfileOptionsConfig = {
  /**
   * Returns a tooltip string when the given profile should render disabled.
   * Used by the kanban task-create dialog for repository-mode executor
   * capability gates.
   */
  disabledReasonFor?: (profile: ExecutorProfile) => string | null;
};

export function useExecutorProfileOptions(
  allProfiles: ExecutorProfile[],
  config?: ExecutorProfileOptionsConfig,
): ExecutorProfileOptionItem[] {
  const disabledReasonFor = config?.disabledReasonFor;
  return useMemo(() => {
    return allProfiles.map((profile) => {
      const Icon = getExecutorIcon(profile.executor_type ?? "local");
      const disabledReason = disabledReasonFor?.(profile) ?? null;
      return {
        value: profile.id,
        label: profile.name,
        executorType: profile.executor_type,
        executorName: profile.executor_name,
        disabled: !!disabledReason,
        disabledReason: disabledReason ?? undefined,
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
  }, [allProfiles, disabledReasonFor]);
}
