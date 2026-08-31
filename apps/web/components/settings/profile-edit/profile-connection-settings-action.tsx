"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconShieldLock } from "@tabler/icons-react";
import { kubernetesExecutorSettingsPath } from "@/hooks/domains/settings/use-kubernetes-settings";
import { useRouter } from "@/lib/routing/client-router";
import type { Executor } from "@/lib/types/http";
import { getExecutorIcon } from "@/lib/executor-icons";

const KubernetesIcon = getExecutorIcon("k8s");

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
        size="sm"
        onClick={() => router.push(`/settings/executors/ssh/${encodeURIComponent(executor.id)}`)}
        className="min-h-11 w-full cursor-pointer sm:w-auto md:min-h-7"
        data-testid="ssh-connection-settings-link"
      >
        <IconShieldLock className="mr-1.5 h-4 w-4" />
        {t("executors:connectionSettings")}
      </Button>
    );
  }
  if (executor.type !== "k8s") return null;
  return (
    <Button
      variant="outline"
      size="sm"
      onClick={() => router.push(kubernetesExecutorSettingsPath(executor.id))}
      className="min-h-11 w-full cursor-pointer sm:w-auto md:min-h-7"
      data-testid="kubernetes-connection-settings-link"
    >
      <KubernetesIcon className="mr-1.5 h-4 w-4" />
      {t("executors:kubernetesClusterConnection")}
    </Button>
  );
}
