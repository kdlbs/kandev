"use client";

import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import type { StorageMaintenanceSettings } from "@/lib/types/system";
import { NumberField, PolicySection, SettingRow } from "./storage-policy-fields";

type Props = {
  settings: StorageMaintenanceSettings;
  savedSettings: StorageMaintenanceSettings;
  dockerAvailable: boolean;
  pending: boolean;
  onChange: (settings: StorageMaintenanceSettings) => void;
};

const defaultDockerNetworkSettings: StorageMaintenanceSettings["docker_networks"] = {
  enabled: false,
  stale_hours: 168,
  quarantine_hours: 24,
  orphan_grace_hours: 1,
  probe_enabled: true,
};

/**
 * Docker-network reclamation policy: the enable switch, the three age/window
 * thresholds, and the capacity probe toggle. Disabled until the operator
 * opts in; analysis runs regardless of the enable state.
 */
export function DockerNetworksPolicySection({
  settings,
  savedSettings,
  dockerAvailable,
  pending,
  onChange,
}: Props) {
  const { t } = useTranslation();
  const current = settings.docker_networks ?? defaultDockerNetworkSettings;
  const saved = savedSettings.docker_networks ?? defaultDockerNetworkSettings;
  const enabledDirty = current.enabled !== saved.enabled;
  const staleHoursDirty = current.stale_hours !== saved.stale_hours;
  const quarantineHoursDirty = current.quarantine_hours !== saved.quarantine_hours;
  const graceDirty = current.orphan_grace_hours !== saved.orphan_grace_hours;
  const probeDirty = current.probe_enabled !== saved.probe_enabled;
  const unavailable = dockerAvailable ? undefined : t("system:storageDockerUnavailable");
  const disabledReason = (pending ? t("system:storageActionPending") : undefined) ?? unavailable;
  const updateNetworks = (docker_networks: StorageMaintenanceSettings["docker_networks"]) =>
    onChange({ ...settings, docker_networks });
  return (
    <PolicySection
      sectionId="docker-networks"
      title={t("system:storageDockerNetworksTitle")}
      description={t("system:storageDockerNetworksDescription")}
      isDirty={enabledDirty || staleHoursDirty || quarantineHoursDirty || graceDirty || probeDirty}
    >
      <SettingRow
        title={t("system:storageDockerNetworksReclaim")}
        description={t("system:storageDockerNetworksReclaimDescription")}
        help={t("system:storageDockerNetworksReclaimHelp")}
        control={
          <Switch
            checked={current.enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(enabled) => updateNetworks({ ...current, enabled })}
            aria-label={t("system:storageDockerNetworksReclaim")}
            data-testid="storage-docker-networks-enabled"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
        <NumberField
          label={t("system:storageDockerNetworksStaleLabel")}
          help={t("system:storageDockerNetworksStaleHelp")}
          value={current.stale_hours}
          min={24}
          max={2562047}
          disabled={Boolean(disabledReason)}
          onChange={(stale_hours) => updateNetworks({ ...current, stale_hours })}
          testId="storage-docker-networks-stale-hours"
          isDirty={staleHoursDirty}
        />
        <NumberField
          label={t("system:storageDockerNetworksQuarantineLabel")}
          help={t("system:storageDockerNetworksQuarantineHelp")}
          value={current.quarantine_hours}
          min={24}
          max={2160}
          disabled={Boolean(disabledReason)}
          onChange={(quarantine_hours) => updateNetworks({ ...current, quarantine_hours })}
          testId="storage-docker-networks-quarantine-hours"
          isDirty={quarantineHoursDirty}
        />
        <NumberField
          label={t("system:storageDockerNetworksGraceLabel")}
          help={t("system:storageDockerNetworksGraceHelp")}
          value={current.orphan_grace_hours}
          min={1}
          max={2160}
          disabled={Boolean(disabledReason)}
          onChange={(orphan_grace_hours) => updateNetworks({ ...current, orphan_grace_hours })}
          testId="storage-docker-networks-grace-hours"
          isDirty={graceDirty}
        />
      </div>
      <SettingRow
        title={t("system:storageDockerNetworksProbe")}
        description={t("system:storageDockerNetworksProbeDescription")}
        help={t("system:storageDockerNetworksProbeHelp")}
        control={
          <Switch
            checked={current.probe_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(probe_enabled) => updateNetworks({ ...current, probe_enabled })}
            aria-label={t("system:storageDockerNetworksProbe")}
            data-testid="storage-docker-networks-probe"
            data-settings-dirty={probeDirty}
          />
        }
      />
      {disabledReason && <p className="pt-2 text-xs text-muted-foreground">{disabledReason}</p>}
    </PolicySection>
  );
}
