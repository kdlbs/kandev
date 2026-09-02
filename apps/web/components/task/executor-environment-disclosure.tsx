"use client";

import { IconRefresh, IconSettings, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

import Link from "@/components/routing/app-link";
import type {
  ContainerLiveStatus,
  SSHLiveStatus,
  TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import {
  executorProfileSettingsPath,
  kubernetesExecutorSettingsPath,
} from "@/lib/settings/executor-settings-routes";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { cn } from "@/lib/utils";
import { EnvironmentInfo } from "./executor-environment-info";

type ExecutorEnvironmentDisclosureProps = {
  env: TaskEnvironment | null;
  container: ContainerLiveStatus | null;
  ssh: SSHLiveStatus | null;
  kubernetes: KubernetesSession | null;
  kubernetesLoaded: boolean;
  kubernetesError: string | null;
  loading: boolean;
  refreshing: boolean;
  isResetting: boolean;
  touch?: boolean;
  onRefresh: () => Promise<void>;
  onReset: () => void;
};

export function ExecutorEnvironmentDisclosure({
  env,
  container,
  ssh,
  kubernetes,
  kubernetesLoaded,
  kubernetesError,
  loading,
  refreshing,
  isResetting,
  touch = false,
  onRefresh,
  onReset,
}: ExecutorEnvironmentDisclosureProps) {
  return (
    <>
      <EnvironmentInfo
        env={env}
        container={container}
        ssh={ssh}
        kubernetes={kubernetes}
        kubernetesLoaded={kubernetesLoaded}
        kubernetesError={kubernetesError}
        loading={loading}
        kubernetesActions={
          env?.executor_type === "k8s" ? (
            <KubernetesActions
              env={env}
              refreshing={refreshing}
              touch={touch}
              onRefresh={onRefresh}
            />
          ) : null
        }
      />
      {env?.executor_type !== "k8s" ? (
        <ResetEnvironmentAction env={env} isResetting={isResetting} onReset={onReset} />
      ) : null}
    </>
  );
}

function KubernetesActions({
  env,
  refreshing,
  touch,
  onRefresh,
}: {
  env: TaskEnvironment;
  refreshing: boolean;
  touch: boolean;
  onRefresh: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const controlClassName = cn(
    "cursor-pointer rounded-md text-muted-foreground hover:bg-muted hover:text-foreground active:scale-[0.96]",
    touch ? "h-11 w-11" : "h-10 w-10",
  );
  const settingsPath = env.executor_profile_id
    ? executorProfileSettingsPath(
        { id: env.executor_id, type: env.executor_type },
        env.executor_profile_id,
      )
    : kubernetesExecutorSettingsPath(env.executor_id);
  return (
    <div
      className="flex shrink-0 items-center gap-0.5"
      data-testid="executor-settings-kubernetes-actions"
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={controlClassName}
            disabled={refreshing}
            aria-label={t("task:refresh")}
            aria-busy={refreshing}
            data-testid="executor-settings-refresh"
            onClick={() => void onRefresh()}
          >
            <IconRefresh
              className={cn("h-4 w-4", refreshing && "animate-spin")}
              data-testid={refreshing ? "executor-settings-refresh-spinner" : undefined}
            />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("task:refresh")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" className={controlClassName} asChild>
            <Link
              href={settingsPath}
              aria-label={t("task:executorSettings")}
              data-testid="executor-settings-link"
            >
              <IconSettings className="h-4 w-4" />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("task:executorSettings")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

function ResetEnvironmentAction({
  env,
  isResetting,
  onReset,
}: {
  env: TaskEnvironment | null;
  isResetting: boolean;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  const hasWorktreePath = Boolean(env?.worktree_path);
  return (
    <div className="flex items-center justify-end border-t border-border px-2 py-1.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={!env || isResetting ? 0 : -1} aria-label={t("task:resetEnvironment2")}>
            <Button
              variant="destructive"
              size="sm"
              className="cursor-pointer text-xs"
              disabled={!env || isResetting}
              data-testid="executor-settings-reset"
              onClick={onReset}
            >
              <IconTrash className="mr-1 h-3.5 w-3.5" />
              {t("task:resetEnvironment2")}
            </Button>
          </span>
        </TooltipTrigger>
        <ResetEnvironmentTooltip hasWorktreePath={hasWorktreePath} />
      </Tooltip>
    </div>
  );
}

function ResetEnvironmentTooltip({ hasWorktreePath }: { hasWorktreePath: boolean }) {
  const { t } = useTranslation();
  return (
    <TooltipContent className="max-w-xs">
      <p className="font-medium">{t("task:resetEnvironment2")}</p>
      <p className="mt-1 text-xs">{t("task:deletesTheCurrentTaskEnvironmentContainer")}</p>
      <p className="mt-1 text-xs text-destructive">
        {t("task:anyUncommittedOrUnpushedChangesAre")}
      </p>
      {hasWorktreePath && (
        <p className="mt-1 text-xs text-muted-foreground">{t("task:pushYourBranchToItsRemote")}</p>
      )}
    </TooltipContent>
  );
}
