"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useKandevRestart } from "@/hooks/domains/system/use-kandev-restart";
import { useRestartCapability } from "@/hooks/domains/system/use-restart-capability";
import { RestartProgressDialog } from "@/components/settings/system/restart-progress-dialog";

export function AgentRuntimeUnavailableAlert() {
  const agentRuntime = useAppStore((state) => state.agentRuntime);
  if (agentRuntime?.status !== "unavailable") return null;

  return <UnavailableAgentRuntimeContent />;
}

function UnavailableAgentRuntimeContent() {
  const { t } = useTranslation();
  const restartCapability = useRestartCapability();
  const restart = useKandevRestart();

  const capabilityLoading = restartCapability.status === "loading";
  const restartSupported =
    restartCapability.status === "resolved" && restartCapability.capability.supported;

  return (
    <>
      <div className="min-w-0 w-full shrink-0 px-3 pt-3 sm:px-4" data-testid="agent-runtime-alert">
        <Alert variant="destructive" className="min-w-0 gap-x-2 px-3 py-3 sm:px-4">
          <IconAlertTriangle className="mt-0.5 size-4" aria-hidden="true" />
          <AlertTitle className="min-w-0 break-words text-sm">
            {t("system:agentRuntimeUnavailableTitle")}
          </AlertTitle>
          <AlertDescription className="col-start-2 flex min-w-0 flex-col gap-3 text-sm sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0 space-y-1 break-words">
              <p>{t("system:agentRuntimeUnavailableBody")}</p>
              <p>{t("system:agentRuntimeUnavailableRestartRequired")}</p>
            </div>
            {restartSupported ? (
              <Button
                type="button"
                size="sm"
                className="h-11 w-full shrink-0 cursor-pointer sm:w-auto"
                disabled={restart.isRestarting}
                onClick={() => void restart.start()}
              >
                {t("system:agentRuntimeUnavailableRestart")}
              </Button>
            ) : (
              <p className="min-w-0 shrink break-words text-sm">
                {capabilityLoading
                  ? t("system:agentRuntimeCheckingRestart")
                  : t("system:agentRuntimeUnavailableManual")}
              </p>
            )}
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
