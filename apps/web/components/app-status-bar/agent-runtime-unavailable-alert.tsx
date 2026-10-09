"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useAgentRuntimeRecovery } from "@/hooks/domains/system/use-agent-runtime-recovery";
import { useKandevRestart } from "@/hooks/domains/system/use-kandev-restart";
import { useRestartCapability } from "@/hooks/domains/system/use-restart-capability";
import { RestartProgressDialog } from "@/components/settings/system/restart-progress-dialog";
import { controlSizingClassName } from "@kandev/ui/control-sizing";

export function AgentRuntimeUnavailableAlert() {
  const agentRuntime = useAppStore((state) => state.agentRuntime);
  if (!agentRuntime || agentRuntime.status === "available") return null;

  return <AgentRuntimeAlertContent recovering={agentRuntime.status === "recovering"} />;
}

function AgentRuntimeAlertContent({ recovering }: { recovering: boolean }) {
  const recovery = useAgentRuntimeRecovery();
  const restartCapability = useRestartCapability();
  const restart = useKandevRestart();
  const recoveryInProgress = recovering || recovery.isRecovering;
  const capabilityLoading = restartCapability.status === "loading";
  const restartSupported =
    restartCapability.status === "resolved" && restartCapability.capability.supported;

  return (
    <>
      <div className="min-w-0 w-full shrink-0 px-3 pt-3 sm:px-4" data-testid="agent-runtime-alert">
        <Alert
          {...getAlertPresentation(recoveryInProgress)}
          className="min-w-0 gap-x-2 px-3 py-3 sm:px-4"
        >
          <AgentRuntimeAlertIndicator recovering={recoveryInProgress} />
          <AgentRuntimeAlertTitle recovering={recoveryInProgress} />
          <AlertDescription className="col-start-2 flex min-w-0 flex-col gap-3 text-sm sm:flex-row sm:items-start sm:justify-between">
            <AgentRuntimeAlertMessage
              recovering={recoveryInProgress}
              isAdmin={recovery.isAdmin}
              error={recovery.error}
              restartSupported={restartSupported}
              capabilityLoading={capabilityLoading}
            />
            <AgentRuntimeAlertActions
              recovering={recoveryInProgress}
              recovery={recovery}
              restart={restart}
              restartSupported={restartSupported}
            />
          </AlertDescription>
        </Alert>
      </div>
      <RestartProgressDialog
        phase={restart.phase}
        errorMessage={restart.errorMessage}
        onDismiss={restart.dismiss}
      />
    </>
  );
}

function getAlertPresentation(recovering: boolean) {
  return {
    role: recovering ? "status" : "alert",
    "aria-live": recovering ? "polite" : "assertive",
    variant: recovering ? "default" : "destructive",
  } as const;
}

function AgentRuntimeAlertIndicator({ recovering }: { recovering: boolean }) {
  return recovering ? (
    <IconLoader2 className="mt-0.5 size-4 animate-spin" aria-hidden="true" />
  ) : (
    <IconAlertTriangle className="mt-0.5 size-4" aria-hidden="true" />
  );
}

function AgentRuntimeAlertTitle({ recovering }: { recovering: boolean }) {
  const { t } = useTranslation();
  return (
    <AlertTitle className="min-w-0 break-words text-sm">
      {recovering
        ? t("system:agentRuntimeRecoveringTitle")
        : t("system:agentRuntimeUnavailableTitle")}
    </AlertTitle>
  );
}

type AlertMessageProps = {
  recovering: boolean;
  isAdmin: boolean;
  error: "stale" | "failed" | null;
  restartSupported: boolean;
  capabilityLoading: boolean;
};

function AgentRuntimeAlertMessage({
  recovering,
  isAdmin,
  error,
  restartSupported,
  capabilityLoading,
}: AlertMessageProps) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-1 break-words">
      <p>
        {recovering
          ? t("system:agentRuntimeRecoveringBody")
          : t("system:agentRuntimeUnavailableBody")}
      </p>
      {!recovering && error && (
        <p role="status" className="text-destructive">
          {error === "stale"
            ? t("system:agentRuntimeRetryStale")
            : t("system:agentRuntimeRetryFailed")}
        </p>
      )}
      {!recovering && !isAdmin && <p>{t("system:agentRuntimeAdminRequired")}</p>}
      {!recovering && isAdmin && !restartSupported && (
        <p>
          {capabilityLoading
            ? t("system:agentRuntimeCheckingRestart")
            : t("system:agentRuntimeUnavailableManual")}
        </p>
      )}
    </div>
  );
}

function AgentRuntimeAlertActions({
  recovering,
  recovery,
  restart,
  restartSupported,
}: {
  recovering: boolean;
  recovery: ReturnType<typeof useAgentRuntimeRecovery>;
  restart: ReturnType<typeof useKandevRestart>;
  restartSupported: boolean;
}) {
  const { t } = useTranslation();
  if (recovering || !recovery.isAdmin) return null;

  return (
    <div className="flex min-w-0 shrink-0 flex-col gap-2 sm:flex-row">
      {recovery.canRetry && (
        <Button
          type="button"
          className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
          disabled={recovery.isRetrying}
          onClick={() => void recovery.retry()}
        >
          {t("system:agentRuntimeRetry")}
        </Button>
      )}
      {restartSupported && (
        <Button
          type="button"
          variant={recovery.canRetry ? "outline" : "default"}
          className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
          disabled={restart.isRestarting}
          onClick={() => void restart.start()}
        >
          {t("system:agentRuntimeUnavailableRestart")}
        </Button>
      )}
    </div>
  );
}
