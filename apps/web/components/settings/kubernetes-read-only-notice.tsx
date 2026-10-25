"use client";

import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { IconLock } from "@tabler/icons-react";

export function KubernetesReadOnlyNotice() {
  const { t } = useTranslation();
  return (
    <Alert data-testid="kubernetes-read-only-notice">
      <IconLock className="h-4 w-4" />
      <AlertTitle>{t("executors:kubernetesReadOnlyTitle")}</AlertTitle>
      <AlertDescription>{t("executors:kubernetesReadOnlyDescription")}</AlertDescription>
    </Alert>
  );
}
