"use client";

import { useState, useCallback } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { IconTrash, IconPlus, IconChevronRight } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { deleteExecutorProfile, listExecutorProfiles } from "@/lib/api/domains/settings-api";
import { ExecutorProfileDialog } from "@/components/settings/executor-profile-dialog";
import { PluginExecutorProfileDialog } from "@/components/settings/plugin-executor-profile-dialog";
import { useAppStore } from "@/components/state-provider";
import type { ExecutorProfile } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { executorProfileSettingsPath } from "@/lib/settings/executor-settings-routes";
import {
  executorProviderUnavailableReason,
  localizedExecutorProviderMessage,
} from "@/lib/executor-provider-display";

type ExecutorProfilesCardProps = {
  executorId: string;
  profiles: ExecutorProfile[];
  title?: string;
  description?: string;
};

const executorProfileDeleteActionClassName = settingsActionClassName(
  "p-0 text-destructive hover:text-destructive cursor-pointer",
);

function ExecutorProfilesHeader({
  title,
  description,
  actionLabel,
  onCreate,
  disabled,
}: {
  title: string;
  description: string;
  actionLabel: string;
  onCreate: () => void;
  disabled: boolean;
}) {
  return (
    <SettingsCardHeader
      title={title}
      description={description}
      actions={
        <Button
          variant="outline"
          size="sm"
          onClick={onCreate}
          disabled={disabled}
          className={settingsActionClassName("cursor-pointer")}
        >
          <IconPlus className="h-4 w-4 mr-1" />
          {actionLabel}
        </Button>
      }
    />
  );
}

function ExecutorProfileRows({
  profiles,
  onOpen,
  onDelete,
}: {
  profiles: ExecutorProfile[];
  onOpen: (profile: ExecutorProfile) => void;
  onDelete: (event: React.MouseEvent, profileId: string) => void;
}) {
  const { t } = useTranslation();
  if (profiles.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("executors:noProfilesConfigured")}</p>;
  }
  return (
    <div className="space-y-2">
      {profiles.map((profile) => (
        <div
          key={profile.id}
          data-testid={`executor-profile-card-${profile.id}`}
          className="flex items-center justify-between rounded-md border px-3 py-2 hover:bg-muted/50 cursor-pointer transition-colors"
          onClick={() => onOpen(profile)}
        >
          <div className="flex items-center gap-2 min-w-0">
            <span className="text-sm font-medium truncate">{profile.name}</span>
            {profile.prepare_script && (
              <Badge variant="outline" className={`${SETTINGS_TYPOGRAPHY.meta} shrink-0`}>
                {t("executors:prepareScriptBadge")}
              </Badge>
            )}
          </div>
          <div className="flex items-center gap-1 flex-shrink-0">
            <Button
              variant="ghost"
              size="icon"
              onClick={(event) => onDelete(event, profile.id)}
              aria-label={t("executors:deleteProfile")}
              data-testid="executor-profile-delete-button"
              className={executorProfileDeleteActionClassName}
            >
              <IconTrash className="h-3.5 w-3.5" />
            </Button>
            <IconChevronRight className="h-4 w-4 text-muted-foreground" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function ExecutorProfilesCard({
  executorId,
  profiles,
  title,
  description,
}: ExecutorProfilesCardProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const [dialogOpen, setDialogOpen] = useState(false);
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const executor = executors.find((item) => item.id === executorId);
  const isPluginExecutor = executor?.type === "plugin_remote";
  const provider = executor?.provider;
  const providerUnavailable = executorProviderUnavailableReason(provider, isPluginExecutor, t);
  const providerName = localizedExecutorProviderMessage(
    provider,
    "display_name",
    provider?.display_name ?? t("executors:profiles"),
    t,
  );
  const providerDescription = localizedExecutorProviderMessage(
    provider,
    "description",
    provider?.description ?? t("executors:differentConfigurationsForThisExecutorEach"),
    t,
  );

  const refreshProfiles = useCallback(async () => {
    try {
      const resp = await listExecutorProfiles(executorId, { cache: "no-store" });
      setExecutors(
        executors.map((e) => (e.id === executorId ? { ...e, profiles: resp.profiles } : e)),
      );
    } catch {
      // ignore refresh failure
    }
  }, [executorId, executors, setExecutors]);

  const handleProfileCreated = useCallback(
    async (profile: ExecutorProfile) => {
      await refreshProfiles();
      router.push(executorProfileSettingsPath(profile.id));
    },
    [refreshProfiles, router],
  );

  const handleDelete = useCallback(
    async (e: React.MouseEvent, profileId: string) => {
      e.stopPropagation();
      try {
        await deleteExecutorProfile(executorId, profileId);
        await refreshProfiles();
      } catch {
        // ignore delete failure
      }
    },
    [executorId, refreshProfiles],
  );

  return (
    <>
      <Card data-testid={`executor-profiles-card-${executorId}`}>
        <ExecutorProfilesHeader
          title={title ?? providerName}
          description={description ?? providerDescription}
          actionLabel={t("executors:add")}
          onCreate={() => setDialogOpen(true)}
          disabled={Boolean(providerUnavailable)}
        />
        <CardContent>
          {providerUnavailable && (
            <p
              role="status"
              className="mb-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-sm"
            >
              {providerUnavailable}
            </p>
          )}
          <ExecutorProfileRows
            profiles={profiles}
            onOpen={(profile) => router.push(executorProfileSettingsPath(profile.id))}
            onDelete={handleDelete}
          />
        </CardContent>
      </Card>
      {isPluginExecutor ? (
        <PluginExecutorProfileDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          executorId={executorId}
          provider={provider}
          onSaved={handleProfileCreated}
        />
      ) : (
        <ExecutorProfileDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          executorId={executorId}
          onSaved={handleProfileCreated}
        />
      )}
    </>
  );
}
