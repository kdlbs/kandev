"use client";

import ExecutorEditPage from "@/app/settings/executor/[id]/page";
import ProfileDetailPage from "@/app/settings/executor/[id]/profile/[profileId]/page";
import { useAppStore } from "@/components/state-provider";
import {
  executorConnectionSettingsPath,
  executorProfileSettingsPath,
} from "@/lib/settings/executor-settings-routes";
import { SettingsRedirect } from "@/src/settings-route-helpers";

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

  if (executor?.type === "k8s") {
    const destination = profileId
      ? executorProfileSettingsPath(executor, profileId)
      : executorConnectionSettingsPath(executor);
    return <SettingsRedirect to={destination} />;
  }

  return profileId ? (
    <ProfileDetailPage executorId={executorId} profileId={profileId} />
  ) : (
    <ExecutorEditPage executorId={executorId} />
  );
}
