"use client";

import { useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { backendReloadCoordinator } from "@/lib/platform/backend-reload-coordinator";

export function BackendReloadRequiredAlert() {
  const { t } = useTranslation();
  const snapshot = useSyncExternalStore(
    backendReloadCoordinator.subscribe,
    backendReloadCoordinator.getSnapshot,
    backendReloadCoordinator.getSnapshot,
  );

  if (!snapshot.reloadRequired || snapshot.ownerCount > 0) return null;

  return (
    <div
      className="min-w-0 w-full shrink-0 px-3 pt-3 sm:px-4"
      data-testid="backend-reload-required-alert"
    >
      <Alert role="alert" variant="destructive" className="min-w-0 gap-x-2 px-3 py-3 sm:px-4">
        <IconAlertTriangle className="mt-0.5 size-4" aria-hidden="true" />
        <AlertTitle className="min-w-0 break-words text-sm">
          {t("system:backendReloadRequiredTitle")}
        </AlertTitle>
        <AlertDescription className="col-start-2 flex min-w-0 flex-col gap-3 text-sm sm:flex-row sm:items-start sm:justify-between">
          <p className="min-w-0 break-words">{t("system:backendReloadRequiredBody")}</p>
          <Button
            type="button"
            size="sm"
            className="h-11 w-full shrink-0 cursor-pointer sm:w-auto"
            onClick={() => window.location.reload()}
          >
            {t("system:backendReloadRequiredAction")}
          </Button>
        </AlertDescription>
      </Alert>
    </div>
  );
}
