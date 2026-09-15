"use client";

import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

export function RemoteProviderConnectionError({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <span
      className="flex max-w-full flex-wrap items-center gap-2 text-xs text-destructive"
      role="alert"
    >
      <span className="min-w-0 break-words">{t("task:repositorySelectionUnavailable")}</span>
      <Button
        type="button"
        variant="outline"
        className="cursor-pointer"
        aria-label={t("task:retryRemoteRepositoryResolution")}
        onClick={onRetry}
      >
        {t("task:retry")}
      </Button>
      <Link
        href="/settings/integrations"
        className="text-foreground underline underline-offset-2 cursor-pointer"
      >
        {t("common:settings")}
      </Link>
    </span>
  );
}
