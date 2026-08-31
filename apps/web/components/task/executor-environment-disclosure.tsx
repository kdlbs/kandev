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
import { kubernetesExecutorSettingsPath } from "@/lib/settings/executor-settings-routes";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { EnvironmentInfo } from "./executor-environment-info";

type ExecutorEnvironmentDisclosureProps = {
  env: TaskEnvironment | null;
  container: ContainerLiveStatus | null;
  ssh: SSHLiveStatus | null;
  kubernetes: KubernetesSession | null;
  kubernetesLoaded: boolean;
  kubernetesError: string | null;
  loading: boolean;
  isResetting: boolean;
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
  isResetting,
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
      />
      {env?.executor_type === "k8s" ? (
        <KubernetesActions env={env} loading={loading} onRefresh={onRefresh} />
      ) : (
        <ResetEnvironmentAction env={env} isResetting={isResetting} onReset={onReset} />
      )}
    </>
  );
}

function KubernetesActions({
  env,
  loading,
  onRefresh,
}: {
  env: TaskEnvironment;
  loading: boolean;
  onRefresh: () => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-end gap-1.5 border-t border-border px-2 py-1.5">
      <Button
        variant="ghost"
        size="sm"
        className="cursor-pointer text-xs"
        disabled={loading}
        data-testid="executor-settings-refresh"
        onClick={() => void onRefresh()}
      >
        <IconRefresh className={`mr-1 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
        {t("task:refresh")}
      </Button>
      <Button variant="outline" size="sm" className="cursor-pointer text-xs" asChild>
        <Link
          href={kubernetesExecutorSettingsPath(env.executor_id)}
          data-testid="executor-settings-link"
        >
          <IconSettings className="mr-1 h-3.5 w-3.5" />
          {t("task:executorSettings")}
        </Link>
      </Button>
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
