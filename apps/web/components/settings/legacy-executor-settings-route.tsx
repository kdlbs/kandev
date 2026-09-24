"use client";

import ExecutorEditPage from "@/app/settings/executor/[id]/page";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import {
  executorConnectionSettingsPath,
  executorProfileSettingsPath,
} from "@/lib/settings/executor-settings-routes";
import type { Executor } from "@/lib/types/http";
import { SettingsRedirect } from "@/src/settings-route-helpers";
import { useTranslation } from "react-i18next";

const EXECUTORS_ROUTE = "/settings/executors";

function ProfileUnavailableState({ executor }: { executor: Executor | null }) {
  const { t } = useTranslation();
  const router = useRouter();
  const destination = executor ? executorConnectionSettingsPath(executor) : EXECUTORS_ROUTE;

  return (
    <Card data-testid="executor-profile-unavailable">
      <CardContent className="py-12 text-center">
        <p className="text-muted-foreground">{t("executors:profileNotFound")}</p>
        <Button className="mt-4 cursor-pointer" onClick={() => router.push(destination)}>
          {executor ? t("executors:backToExecutor") : t("executors:backToExecutors")}
        </Button>
      </CardContent>
    </Card>
  );
}

export function LegacyExecutorSettingsRoute({
  executorId,
  profileId,
}: {
  executorId: string;
  profileId?: string;
}) {
  const executor = useAppStore(
    (state) => state.executors.items.find((item) => item.id === executorId) ?? null,
  );
  const executorsLoaded = useAppStore((state) => state.settingsData.executorsLoaded);

  if (profileId !== undefined) {
    if (!executorsLoaded) return null;
    const profile = executor?.profiles?.find(
      (candidate) => candidate.id === profileId && candidate.executor_id === executorId,
    );
    if (!executor || !profile) return <ProfileUnavailableState executor={executor} />;
    return <SettingsRedirect to={executorProfileSettingsPath(profile.id)} />;
  }

  if (executor?.type === "k8s") {
    const destination = executor.profiles?.[0]?.id
      ? executorProfileSettingsPath(executor.profiles[0].id)
      : executorConnectionSettingsPath(executor);
    return <SettingsRedirect to={destination} />;
  }

  return <ExecutorEditPage executorId={executorId} />;
}
