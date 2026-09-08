"use client";

import { useEffect, useRef, useState } from "react";
import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";
import { settingsWithDockerAcknowledgement } from "@/hooks/domains/system/use-storage-maintenance";
import type { StorageCapabilities, StorageMaintenanceSettings } from "@/lib/types/system";
import { DedicatedDockerDialog, ExternalGoCacheDialog } from "./storage-confirmation-dialogs";
import { StorageAdoptionField } from "./storage-adoption-field";
import { NumberField, PolicySection, SettingRow } from "./storage-policy-fields";
import { bytesToGigabytes, gigabytesToBytes } from "./storage-units";
import { StorageWorkspaceDependencySettings } from "./storage-workspace-dependency-settings";

type Props = {
  settings: StorageMaintenanceSettings;
  savedSettings: StorageMaintenanceSettings;
  capabilities: StorageCapabilities;
  pending: boolean;
  /**
   * Why the controls are disabled, when it is not the default "an action is
   * already running". The admin gate uses it so a member sees the real reason.
   */
  pendingReason?: string;
  onChange: (settings: StorageMaintenanceSettings) => void;
  onAdopt: (path: string) => Promise<void>;
  onCleanDependencies?: () => void;
};

type PolicySectionProps = Pick<
  Props,
  "settings" | "savedSettings" | "capabilities" | "onChange" | "pending" | "pendingReason"
>;

function settingIsDirty<T>(
  settings: StorageMaintenanceSettings,
  savedSettings: StorageMaintenanceSettings,
  select: (value: StorageMaintenanceSettings) => T,
): boolean {
  return !Object.is(select(settings), select(savedSettings));
}

function ScheduleSection({ settings, savedSettings, pending, onChange }: PolicySectionProps) {
  const { t } = useTranslation();
  const enabledDirty = settingIsDirty(settings, savedSettings, (value) => value.enabled);
  const intervalDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.check_interval_hours,
  );
  const idleDirty = settingIsDirty(settings, savedSettings, (value) => value.idle_for_minutes);
  return (
    <PolicySection
      sectionId="schedule"
      title={t("system:storageScheduleTitle")}
      description={t("system:storageScheduleDescription")}
      isDirty={enabledDirty || intervalDirty || idleDirty}
    >
      <SettingRow
        title={t("system:storageScheduledMaintenance")}
        description={t("system:storageScheduledMaintenanceDescription")}
        help={t("system:storageScheduledMaintenanceHelp")}
        control={
          <Switch
            checked={settings.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, enabled })}
            aria-label={t("system:storageScheduledMaintenance")}
            data-testid="storage-scheduling-enabled"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t("system:storageCheckIntervalLabel")}
          help={t("system:storageCheckIntervalHelp")}
          value={settings.check_interval_hours}
          min={1}
          max={168}
          disabled={pending || !settings.enabled}
          onChange={(check_interval_hours) => onChange({ ...settings, check_interval_hours })}
          testId="storage-check-interval"
          isDirty={intervalDirty}
        />
        <NumberField
          label={t("system:storageIdleLabel")}
          help={t("system:storageIdleHelp")}
          value={settings.idle_for_minutes}
          min={1}
          max={1440}
          disabled={pending || !settings.enabled}
          onChange={(idle_for_minutes) => onChange({ ...settings, idle_for_minutes })}
          testId="storage-idle-period"
          isDirty={idleDirty}
        />
      </div>
    </PolicySection>
  );
}

function WorkspaceSection({
  settings,
  savedSettings,
  pending,
  pendingReason,
  onChange,
  onCleanDependencies,
}: PolicySectionProps & Pick<Props, "onCleanDependencies">) {
  const { t } = useTranslation();
  const workspacesDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.workspaces.enabled,
  );
  const graceDirty = settingIsDirty(settings, savedSettings, (value) => value.orphan_grace_hours);
  const containersDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.kandev_containers.enabled,
  );
  const dependencyCleanupDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.workspaces.dependency_cleanup_enabled,
  );
  return (
    <PolicySection
      sectionId="workspaces"
      title={t("system:storageWorkspacesTitle")}
      description={t("system:storageWorkspacesDescription")}
      isDirty={workspacesDirty || graceDirty || dependencyCleanupDirty || containersDirty}
    >
      <SettingRow
        title={t("system:storageOrphanWorkspaces")}
        description={t("system:storageOrphanWorkspacesDescription")}
        help={t("system:storageOrphanWorkspacesHelp")}
        control={
          <Switch
            checked={settings.workspaces.enabled}
            disabled={pending}
            onCheckedChange={(enabled) =>
              onChange({ ...settings, workspaces: { ...settings.workspaces, enabled } })
            }
            aria-label={t("system:storageCleanOrphanWorkspacesAria")}
            data-settings-dirty={workspacesDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
        <NumberField
          label={t("system:storageOrphanGraceLabel")}
          help={t("system:storageOrphanGraceHelp")}
          value={settings.orphan_grace_hours}
          min={24}
          max={2160}
          disabled={pending || !settings.workspaces.enabled}
          onChange={(orphan_grace_hours) => onChange({ ...settings, orphan_grace_hours })}
          testId="storage-orphan-grace"
          isDirty={graceDirty}
        />
      </div>
      <StorageWorkspaceDependencySettings
        settings={settings}
        savedSettings={savedSettings}
        pending={pending}
        pendingReason={pendingReason}
        onChange={onChange}
        onCleanDependencies={onCleanDependencies}
      />
      <SettingRow
        title={t("system:storageKandevContainers")}
        description={t("system:storageContainersDescription")}
        help={t("system:storageContainersHelp")}
        control={
          <Switch
            checked={settings.kandev_containers.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, kandev_containers: { enabled } })}
            aria-label={t("system:storageCleanContainersAria")}
            data-settings-dirty={containersDirty}
          />
        }
      />
    </PolicySection>
  );
}

function GoCacheSection({
  settings,
  savedSettings,
  capabilities,
  pending,
  pendingReason,
  onChange,
  adoptionPath,
  setAdoptionPath,
  onOpenAdoption,
}: PolicySectionProps & {
  adoptionPath: string;
  setAdoptionPath: (path: string) => void;
  onOpenAdoption: () => void;
}) {
  const { t } = useTranslation();
  const enabledDirty = settingIsDirty(settings, savedSettings, (value) => value.go_cache.enabled);
  const maxBytesDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.go_cache.max_bytes,
  );
  return (
    <PolicySection
      sectionId="go-cache"
      title={t("system:storageGoBuildCache")}
      description={t("system:storageGoCacheSectionDescription")}
      isDirty={enabledDirty || maxBytesDirty}
    >
      <SettingRow
        title={t("system:storageManagedGoCache")}
        description={t("system:storageManagedGoCacheDescription", {
          path: capabilities.managed_go_cache_path,
        })}
        help={t("system:storageManagedGoCacheHelp")}
        control={
          <Switch
            checked={settings.go_cache.enabled}
            disabled={pending}
            onCheckedChange={(enabled) =>
              onChange({ ...settings, go_cache: { ...settings.go_cache, enabled } })
            }
            aria-label={t("system:storageEnableManagedGoCacheAria")}
            data-testid="storage-go-cache-enabled"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t("system:storageGoCacheMaxLabel")}
          help={t("system:storageGoCacheMaxHelp")}
          value={bytesToGigabytes(settings.go_cache.max_bytes)}
          min={1}
          disabled={pending || !settings.go_cache.enabled}
          onChange={(gigabytes) =>
            onChange({
              ...settings,
              go_cache: { ...settings.go_cache, max_bytes: gigabytesToBytes(gigabytes) },
            })
          }
          testId="storage-go-cache-max"
          isDirty={maxBytesDirty}
        />
      </div>
      {capabilities.go_cache_adoption_available && (
        <StorageAdoptionField
          path={adoptionPath}
          setPath={setAdoptionPath}
          pending={pending}
          pendingReason={pendingReason}
          enabled={settings.go_cache.enabled}
          onOpen={onOpenAdoption}
        />
      )}
    </PolicySection>
  );
}

type DockerSettings = StorageMaintenanceSettings["docker"];

function DockerBuildCacheSettings({
  docker,
  savedDocker,
  disabledReason,
  updateDocker,
}: {
  docker: DockerSettings;
  savedDocker: DockerSettings;
  disabledReason?: string;
  updateDocker: (docker: DockerSettings) => void;
}) {
  const { t } = useTranslation();
  const enabledDirty = docker.build_cache_enabled !== savedDocker.build_cache_enabled;
  const keepBytesDirty = docker.build_cache_keep_bytes !== savedDocker.build_cache_keep_bytes;
  const unusedHoursDirty = docker.build_cache_unused_hours !== savedDocker.build_cache_unused_hours;
  return (
    <>
      <SettingRow
        title={t("system:storageDockerBuildCache")}
        description={t("system:storageDockerBuildCacheDescription")}
        help={t("system:storageDockerBuildCacheHelp")}
        control={
          <Switch
            checked={docker.build_cache_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(build_cache_enabled) =>
              updateDocker({ ...docker, build_cache_enabled })
            }
            aria-label={t("system:storageCleanDockerBuildCacheAria")}
            data-testid="storage-docker-build-cache"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
        <NumberField
          label={t("system:storageBuildCacheKeepLabel")}
          help={t("system:storageBuildCacheKeepHelp")}
          value={bytesToGigabytes(docker.build_cache_keep_bytes)}
          min={1}
          disabled={Boolean(disabledReason) || !docker.build_cache_enabled}
          onChange={(gigabytes) =>
            updateDocker({
              ...docker,
              build_cache_keep_bytes: gigabytesToBytes(gigabytes),
            })
          }
          testId="storage-docker-build-cache-keep-bytes"
          isDirty={keepBytesDirty}
        />
        <NumberField
          label={t("system:storageBuildCacheUnusedLabel")}
          help={t("system:storageBuildCacheUnusedHelp")}
          value={docker.build_cache_unused_hours}
          min={24}
          max={2562047}
          disabled={Boolean(disabledReason) || !docker.build_cache_enabled}
          onChange={(build_cache_unused_hours) =>
            updateDocker({ ...docker, build_cache_unused_hours })
          }
          testId="storage-docker-build-cache-unused-hours"
          isDirty={unusedHoursDirty}
        />
      </div>
    </>
  );
}

function DockerImageSettings({
  docker,
  savedDocker,
  disabledReason,
  updateDocker,
}: {
  docker: DockerSettings;
  savedDocker: DockerSettings;
  disabledReason?: string;
  updateDocker: (docker: DockerSettings) => void;
}) {
  const { t } = useTranslation();
  const enabledDirty = docker.unused_images_enabled !== savedDocker.unused_images_enabled;
  const hoursDirty = docker.unused_images_hours !== savedDocker.unused_images_hours;
  return (
    <>
      <SettingRow
        title={t("system:storageUnusedDockerImages")}
        description={t("system:storageUnusedImagesDescription")}
        help={t("system:storageUnusedImagesHelp")}
        control={
          <Switch
            checked={docker.unused_images_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(unused_images_enabled) =>
              updateDocker({ ...docker, unused_images_enabled })
            }
            aria-label={t("system:storageCleanUnusedImagesAria")}
            data-testid="storage-docker-unused-images"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t("system:storageUnusedImagesHoursLabel")}
          help={t("system:storageUnusedImagesHoursHelp")}
          value={docker.unused_images_hours}
          min={24}
          max={2562047}
          disabled={Boolean(disabledReason) || !docker.unused_images_enabled}
          onChange={(unused_images_hours) => updateDocker({ ...docker, unused_images_hours })}
          testId="storage-docker-unused-images-hours"
          isDirty={hoursDirty}
        />
      </div>
    </>
  );
}

function DockerSection({
  settings,
  savedSettings,
  capabilities,
  pending,
  pendingReason,
  onChange,
  onOpenDedicated,
}: PolicySectionProps & { onOpenDedicated: () => void }) {
  const { t } = useTranslation();
  const dockerDirty = JSON.stringify(settings.docker) !== JSON.stringify(savedSettings.docker);
  const dedicatedDirty =
    settings.docker.dedicated_daemon_acknowledged !==
    savedSettings.docker.dedicated_daemon_acknowledged;
  const unavailable = capabilities.docker_available
    ? undefined
    : t("system:storageDockerUnavailable");
  const disabledReason =
    (pending ? (pendingReason ?? t("system:storageActionPending")) : undefined) ??
    unavailable ??
    (!settings.docker.dedicated_daemon_acknowledged
      ? t("system:storageAcknowledgeDedicatedFirst")
      : undefined);
  const updateDocker = (docker: StorageMaintenanceSettings["docker"]) =>
    onChange({ ...settings, docker });
  return (
    <PolicySection
      sectionId="docker"
      title={t("system:storageDockerTitle")}
      description={t("system:storageDockerDescription")}
      isDirty={dockerDirty}
    >
      <SettingRow
        title={t("system:storageDedicatedDaemon")}
        description={t("system:storageDedicatedDaemonDescription")}
        help={t("system:storageDedicatedDaemonHelp")}
        control={
          <Switch
            checked={settings.docker.dedicated_daemon_acknowledged}
            disabled={pending || !capabilities.docker_available}
            onCheckedChange={(checked) => {
              if (checked) onOpenDedicated();
              else onChange(settingsWithDockerAcknowledgement(settings, false));
            }}
            aria-label={t("system:storageDedicatedDaemon")}
            data-testid="storage-docker-dedicated"
            data-settings-dirty={dedicatedDirty}
          />
        }
      />
      {unavailable && (
        <p className="py-2 text-xs text-amber-600">{t("system:storageDockerUnavailableNotice")}</p>
      )}
      <DockerBuildCacheSettings
        docker={settings.docker}
        savedDocker={savedSettings.docker}
        disabledReason={disabledReason}
        updateDocker={updateDocker}
      />
      <DockerImageSettings
        docker={settings.docker}
        savedDocker={savedSettings.docker}
        disabledReason={disabledReason}
        updateDocker={updateDocker}
      />
      {disabledReason && <p className="pt-2 text-xs text-muted-foreground">{disabledReason}</p>}
    </PolicySection>
  );
}

function QuarantineSection({ settings, savedSettings, pending, onChange }: PolicySectionProps) {
  const { t } = useTranslation();
  const retentionDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.quarantine_retention_hours,
  );
  return (
    <PolicySection
      sectionId="quarantine"
      title={t("system:storageQuarantineSafetyTitle")}
      description={t("system:storageQuarantineSafetyDescription")}
      isDirty={retentionDirty}
    >
      <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
        <NumberField
          label={t("system:storageQuarantineRetentionLabel")}
          help={t("system:storageQuarantineRetentionHelp")}
          value={settings.quarantine_retention_hours}
          min={24}
          max={2160}
          disabled={pending}
          onChange={(quarantine_retention_hours) =>
            onChange({ ...settings, quarantine_retention_hours })
          }
          testId="storage-quarantine-retention"
          isDirty={retentionDirty}
        />
      </div>
    </PolicySection>
  );
}

export function StoragePolicyCard({
  settings,
  savedSettings,
  capabilities,
  pending,
  pendingReason,
  onChange,
  onAdopt,
  onCleanDependencies,
}: Props) {
  const { t } = useTranslation();
  const [dockerDialogOpen, setDockerDialogOpen] = useState(false);
  const [adoptionDialogOpen, setAdoptionDialogOpen] = useState(false);
  const savedAdoptionPath = savedSettings.go_cache.adopted_path;
  const [adoptionPath, setAdoptionPath] = useState(savedAdoptionPath);
  const previousSavedAdoptionPath = useRef(savedAdoptionPath);

  useEffect(() => {
    const previousPath = previousSavedAdoptionPath.current;
    setAdoptionPath((currentPath) =>
      currentPath === previousPath ? savedAdoptionPath : currentPath,
    );
    previousSavedAdoptionPath.current = savedAdoptionPath;
  }, [savedAdoptionPath]);

  return (
    <section className="min-w-0 space-y-4" data-testid="storage-policy-card">
      <div>
        <h2 className={SETTINGS_TYPOGRAPHY.sectionTitle}>{t("system:storagePolicyTitle")}</h2>
        <p className={SETTINGS_TYPOGRAPHY.sectionDescription}>
          {t("system:storagePolicyDescription")}
        </p>
      </div>
      <div className="space-y-3">
        <ScheduleSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          pendingReason={pendingReason}
          onChange={onChange}
        />
        <WorkspaceSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          pendingReason={pendingReason}
          onChange={onChange}
          onCleanDependencies={onCleanDependencies}
        />
        <GoCacheSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          pendingReason={pendingReason}
          onChange={onChange}
          adoptionPath={adoptionPath}
          setAdoptionPath={setAdoptionPath}
          onOpenAdoption={() => setAdoptionDialogOpen(true)}
        />
        <DockerSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          pendingReason={pendingReason}
          onChange={onChange}
          onOpenDedicated={() => setDockerDialogOpen(true)}
        />
        <QuarantineSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          pendingReason={pendingReason}
          onChange={onChange}
        />
      </div>
      <DedicatedDockerDialog
        open={dockerDialogOpen}
        onOpenChange={setDockerDialogOpen}
        onConfirm={() => {
          const next = settingsWithDockerAcknowledgement(settings, true);
          onChange(next);
          setDockerDialogOpen(false);
        }}
      />
      <ExternalGoCacheDialog
        path={adoptionPath}
        open={adoptionDialogOpen}
        onOpenChange={setAdoptionDialogOpen}
        onConfirm={() => {
          void onAdopt(adoptionPath.trim());
          setAdoptionDialogOpen(false);
        }}
      />
    </section>
  );
}
