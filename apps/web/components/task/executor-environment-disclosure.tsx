"use client";

import { IconRefresh, IconSettings, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

import Link from "@/components/routing/app-link";
import type {
  ContainerLiveStatus,
  PluginExecutorLiveStatus,
  SSHLiveStatus,
  TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import {
  executorProfileSettingsPath,
  kubernetesExecutorSettingsPath,
} from "@/lib/settings/executor-settings-routes";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { cn } from "@/lib/utils";
import { formatDateTime } from "@/lib/i18n/formats";
import { resolvePluginExecutorRetention } from "./executor-environment-status";
import { EnvironmentInfo } from "./executor-environment-info";

const EXECUTOR_SETTINGS_REFRESH_TEST_ID = "executor-settings-refresh";
const EXECUTOR_SETTINGS_LINK_TEST_ID = "executor-settings-link";

type ExecutorEnvironmentDisclosureProps = {
  env: TaskEnvironment | null;
  container: ContainerLiveStatus | null;
  ssh: SSHLiveStatus | null;
  pluginExecutor?: PluginExecutorLiveStatus | null;
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
  pluginExecutor,
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
        pluginExecutor={pluginExecutor}
      />
      {env?.executor_type === "plugin_remote" && pluginExecutor ? (
        <PluginExecutorStatusDetails status={pluginExecutor} />
      ) : null}
      {env?.executor_type === "plugin_remote" ? (
        <PluginExecutorActions
          env={env}
          refreshing={refreshing}
          touch={touch}
          onRefresh={onRefresh}
        />
      ) : null}
      {env?.executor_type !== "k8s" ? (
        <ResetEnvironmentAction
          env={env}
          isResetting={isResetting}
          touch={touch}
          retryCleanup={pluginExecutor?.state === "cleanup_pending"}
          onReset={onReset}
        />
      ) : null}
    </>
  );
}

function PluginExecutorActions({
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
  const refreshLabel = t("task:refresh");
  const settingsLabel = t("task:executorSettings");
  const controlClassName = cn(
    "cursor-pointer rounded-md text-muted-foreground hover:bg-muted hover:text-foreground active:scale-[0.96]",
    touch ? "h-11 w-11" : "h-10 w-10",
  );
  const settingsPath = env.executor_profile_id
    ? executorProfileSettingsPath(env.executor_profile_id)
    : "/settings/executors";
  return (
    <div
      className="flex items-center justify-end gap-1 px-2 py-1"
      data-testid="plugin-executor-actions"
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={controlClassName}
            disabled={refreshing}
            aria-label={refreshLabel}
            aria-busy={refreshing}
            data-testid={EXECUTOR_SETTINGS_REFRESH_TEST_ID}
            onClick={() => void onRefresh()}
          >
            <IconRefresh className={cn("h-4 w-4", refreshing && "animate-spin")} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{refreshLabel}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" className={controlClassName} asChild>
            <Link
              href={settingsPath}
              aria-label={settingsLabel}
              data-testid={EXECUTOR_SETTINGS_LINK_TEST_ID}
            >
              <IconSettings className="h-4 w-4" />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{settingsLabel}</TooltipContent>
      </Tooltip>
    </div>
  );
}

function PluginExecutorStatusDetails({ status }: { status: PluginExecutorLiveStatus }) {
  const { t } = useTranslation();
  const descriptionKey = pluginExecutorDescriptionKey(status);
  return (
    <section
      className="space-y-2 border-t border-border px-3 py-3"
      data-testid="plugin-executor-status"
    >
      {descriptionKey ? (
        <p className="text-xs text-muted-foreground" data-testid="plugin-executor-status-message">
          {t(descriptionKey)}
        </p>
      ) : null}
      <dl className="grid grid-cols-1 gap-2 text-xs">
        <div>
          <dt className="font-medium text-muted-foreground">
            {t("task:pluginExecutorRetentionLabel")}
          </dt>
          <dd className="mt-0.5 text-foreground" data-testid="plugin-executor-retention">
            {resolvePluginExecutorRetention(status.retention)}
          </dd>
        </div>
        {status.expires_at ? (
          <div>
            <dt className="font-medium text-muted-foreground">
              {t("task:pluginExecutorExpiresAt")}
            </dt>
            <dd
              className="mt-0.5 tabular-nums text-foreground"
              data-testid="plugin-executor-expires-at"
            >
              {formatDateTime(status.expires_at)}
            </dd>
          </div>
        ) : null}
      </dl>
    </section>
  );
}

function pluginExecutorDescriptionKey(status: PluginExecutorLiveStatus): string | null {
  switch (status.state) {
    case "expired":
      return "task:pluginExecutorExpiredDescription";
    case "cleanup_pending":
      return "task:pluginExecutorCleanupPendingDescription";
    case "unavailable":
      return "task:pluginExecutorUnavailableDescription";
    case "unknown":
      return "task:pluginExecutorUnknownDescription";
    default:
      return status.deadline_passed ? "task:pluginExecutorDeadlinePassedDescription" : null;
  }
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
    ? executorProfileSettingsPath(env.executor_profile_id)
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
            data-testid={EXECUTOR_SETTINGS_REFRESH_TEST_ID}
            onClick={() => void onRefresh()}
          >
            <IconRefresh
              className={cn("h-4 w-4", refreshing && "animate-spin")}
              data-testid={refreshing ? `${EXECUTOR_SETTINGS_REFRESH_TEST_ID}-spinner` : undefined}
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
              data-testid={EXECUTOR_SETTINGS_LINK_TEST_ID}
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
  touch,
  retryCleanup,
  onReset,
}: {
  env: TaskEnvironment | null;
  isResetting: boolean;
  touch: boolean;
  retryCleanup: boolean;
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
              className={cn("cursor-pointer text-xs", touch && "min-h-11 px-3")}
              disabled={!env || isResetting}
              data-testid="executor-settings-reset"
              onClick={onReset}
            >
              <IconTrash className="mr-1 h-3.5 w-3.5" />
              {retryCleanup ? t("task:pluginExecutorRetryCleanup") : t("task:resetEnvironment2")}
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
