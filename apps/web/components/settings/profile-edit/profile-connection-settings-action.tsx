"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconShieldLock } from "@tabler/icons-react";
import { useRouter } from "@/lib/routing/client-router";
import type { Executor } from "@/lib/types/http";
import { settingsActionClassName } from "@/components/settings/settings-control";

export function ProfileConnectionSettingsAction({
  executor,
}: {
  executor: Pick<Executor, "id" | "type">;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  if (executor.type === "ssh") {
    return (
      <Button
        variant="outline"
        onClick={() => router.push(`/settings/executors/ssh/${encodeURIComponent(executor.id)}`)}
        className={settingsActionClassName("w-full cursor-pointer sm:w-auto")}
        data-testid="ssh-connection-settings-link"
      >
        <IconShieldLock className="mr-1.5 h-4 w-4" />
        {t("executors:connectionSettings")}
      </Button>
    );
  }
  return null;
}
