"use client";

import { useState, type ReactNode } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { StorageCapabilities, StorageMaintenanceSettings } from "@/lib/types/system";
import { settingsWithDockerAcknowledgement } from "@/hooks/domains/system/use-storage-maintenance";
import { DedicatedDockerDialog, ExternalGoCacheDialog } from "./storage-confirmation-dialogs";
import { StorageActionButton } from "./storage-action-button";

type Props = {
  settings: StorageMaintenanceSettings;
  capabilities: StorageCapabilities;
  pending: boolean;
  onChange: (settings: StorageMaintenanceSettings) => void;
  onSave: () => Promise<void>;
  onDedicatedConfirm: (settings: StorageMaintenanceSettings) => Promise<void>;
  onAdopt: (path: string) => Promise<void>;
};

function SettingRow({
  title,
  description,
  control,
}: {
  title: string;
  description: string;
  control: ReactNode;
}) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-4 border-b py-3 last:border-b-0">
      <div className="min-w-0">
        <Label className="text-sm">{title}</Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <div className="shrink-0">{control}</div>
    </div>
  );
}

function NumberField({
  label,
  value,
  min,
  max,
  disabled,
  onChange,
  testId,
}: {
  label: string;
  value: number;
  min: number;
  max?: number;
  disabled?: boolean;
  onChange: (value: number) => void;
  testId: string;
}) {
  return (
    <label className="min-w-0 space-y-1 text-xs text-muted-foreground">
      <span>{label}</span>
      <Input
        type="number"
        min={min}
        max={max}
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        className="h-11"
        data-testid={testId}
      />
    </label>
  );
}

type PolicySectionProps = Pick<Props, "settings" | "capabilities" | "onChange" | "pending">;

function PolicySwitches({ settings, capabilities, pending, onChange }: PolicySectionProps) {
  return (
    <div>
      <SettingRow
        title="Scheduled maintenance"
        description="Disabled by default; runs only after the configured idle period."
        control={
          <Switch
            checked={settings.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, enabled })}
            aria-label="Scheduled maintenance"
            data-testid="storage-scheduling-enabled"
          />
        }
      />
      <SettingRow
        title="Orphan task workspaces"
        description="Move positively identified orphan task roots to quarantine."
        control={
          <Switch
            checked={settings.workspaces.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, workspaces: { enabled } })}
            aria-label="Clean orphan task workspaces"
          />
        }
      />
      <SettingRow
        title="Kandev containers"
        description="Remove stopped managed containers only after inventory confirms they are unused."
        control={
          <Switch
            checked={settings.kandev_containers.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, kandev_containers: { enabled } })}
            aria-label="Clean Kandev containers"
          />
        }
      />
      <SettingRow
        title="Managed Go cache"
        description={`New local executions use ${capabilities.managed_go_cache_path}.`}
        control={
          <Switch
            checked={settings.go_cache.enabled}
            disabled={pending}
            onCheckedChange={(enabled) =>
              onChange({ ...settings, go_cache: { ...settings.go_cache, enabled } })
            }
            aria-label="Enable managed Go cache"
            data-testid="storage-go-cache-enabled"
          />
        }
      />
    </div>
  );
}

function CorePolicySection({ settings, capabilities, pending, onChange }: PolicySectionProps) {
  const setNumber = (
    key:
      | "check_interval_hours"
      | "idle_for_minutes"
      | "orphan_grace_hours"
      | "quarantine_retention_hours",
    value: number,
  ) => onChange({ ...settings, [key]: value });
  return (
    <>
      <PolicySwitches
        settings={settings}
        capabilities={capabilities}
        pending={pending}
        onChange={onChange}
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
        <NumberField
          label="Check interval (hours)"
          value={settings.check_interval_hours}
          min={1}
          max={168}
          disabled={pending}
          onChange={(value) => setNumber("check_interval_hours", value)}
          testId="storage-check-interval"
        />
        <NumberField
          label="Idle period (minutes)"
          value={settings.idle_for_minutes}
          min={1}
          max={1440}
          disabled={pending}
          onChange={(value) => setNumber("idle_for_minutes", value)}
          testId="storage-idle-period"
        />
        <NumberField
          label="Orphan grace (hours)"
          value={settings.orphan_grace_hours}
          min={24}
          max={2160}
          disabled={pending}
          onChange={(value) => setNumber("orphan_grace_hours", value)}
          testId="storage-orphan-grace"
        />
        <NumberField
          label="Quarantine retention (hours)"
          value={settings.quarantine_retention_hours}
          min={24}
          max={2160}
          disabled={pending}
          onChange={(value) => setNumber("quarantine_retention_hours", value)}
          testId="storage-quarantine-retention"
        />
        <NumberField
          label="Go cache maximum (bytes)"
          value={settings.go_cache.max_bytes}
          min={1073741824}
          disabled={pending}
          onChange={(value) =>
            onChange({ ...settings, go_cache: { ...settings.go_cache, max_bytes: value } })
          }
          testId="storage-go-cache-max"
        />
      </div>
    </>
  );
}

function AdoptionSection({
  path,
  setPath,
  onOpen,
  pending,
}: {
  path: string;
  setPath: (path: string) => void;
  onOpen: () => void;
  pending: boolean;
}) {
  let disabledReason: string | undefined;
  if (pending) disabledReason = "Wait for the current storage action to finish.";
  else if (!path.trim()) disabledReason = "Enter an absolute cache path first.";
  return (
    <div className="min-w-0 space-y-2 rounded-md border p-3">
      <Label htmlFor="storage-adoption-path">External Go-cache path</Label>
      <p className="text-xs text-muted-foreground">
        Adoption grants Kandev permission to rotate this existing cache.
      </p>
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
        <Input
          id="storage-adoption-path"
          value={path}
          disabled={pending}
          onChange={(event) => setPath(event.target.value)}
          placeholder="/root/.cache/go-build"
          className="h-11 min-w-0 font-mono"
          data-testid="storage-go-cache-adopt-path"
        />
        <StorageActionButton
          variant="outline"
          disabledReason={disabledReason}
          onClick={onOpen}
          data-testid="storage-go-cache-adopt"
        >
          Adopt cache
        </StorageActionButton>
      </div>
    </div>
  );
}

function DockerThresholdFields({
  settings,
  pending,
  onChange,
}: Pick<PolicySectionProps, "settings" | "pending" | "onChange">) {
  const setDockerNumber = (
    key: "build_cache_keep_bytes" | "build_cache_unused_hours" | "unused_images_hours",
    value: number,
  ) => onChange({ ...settings, docker: { ...settings.docker, [key]: value } });
  return (
    <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
      <NumberField
        label="Build cache retained (bytes)"
        value={settings.docker.build_cache_keep_bytes}
        min={1073741824}
        disabled={pending}
        onChange={(value) => setDockerNumber("build_cache_keep_bytes", value)}
        testId="storage-docker-build-cache-keep-bytes"
      />
      <NumberField
        label="Build cache unused age (hours)"
        value={settings.docker.build_cache_unused_hours}
        min={24}
        max={2562047}
        disabled={pending}
        onChange={(value) => setDockerNumber("build_cache_unused_hours", value)}
        testId="storage-docker-build-cache-unused-hours"
      />
      <NumberField
        label="Unused image age (hours)"
        value={settings.docker.unused_images_hours}
        min={24}
        max={2562047}
        disabled={pending}
        onChange={(value) => setDockerNumber("unused_images_hours", value)}
        testId="storage-docker-unused-images-hours"
      />
    </div>
  );
}

function DockerPolicySection({
  settings,
  capabilities,
  pending,
  onChange,
  onOpen,
}: PolicySectionProps & { onOpen: () => void }) {
  const unavailable = capabilities.docker_available
    ? undefined
    : "Docker is unavailable on the configured host.";
  const disabledReason =
    (pending ? "Wait for the current storage action to finish." : undefined) ??
    unavailable ??
    (!settings.docker.dedicated_daemon_acknowledged
      ? "Acknowledge a dedicated Docker daemon first."
      : undefined);
  return (
    <div className="space-y-1 rounded-md border p-3">
      <SettingRow
        title="Dedicated Docker daemon"
        description="Required for host-global build-cache and unused-image cleanup."
        control={
          <Switch
            checked={settings.docker.dedicated_daemon_acknowledged}
            disabled={pending || !capabilities.docker_available}
            onCheckedChange={(checked) => {
              if (checked) onOpen();
              else onChange(settingsWithDockerAcknowledgement(settings, false));
            }}
            aria-label="Dedicated Docker daemon"
            data-testid="storage-docker-dedicated"
          />
        }
      />
      {unavailable && (
        <p className="text-xs text-amber-600">
          Docker unavailable: daemon-wide actions remain disabled.
        </p>
      )}
      <SettingRow
        title="Docker build cache"
        description="Uses Docker API age and storage filters; never runs system prune."
        control={
          <Switch
            checked={settings.docker.build_cache_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(enabled) =>
              onChange({
                ...settings,
                docker: { ...settings.docker, build_cache_enabled: enabled },
              })
            }
            aria-label="Clean Docker build cache"
            data-testid="storage-docker-build-cache"
          />
        }
      />
      <SettingRow
        title="Unused Docker images"
        description="Only images unused by every container and older than the configured age."
        control={
          <Switch
            checked={settings.docker.unused_images_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(enabled) =>
              onChange({
                ...settings,
                docker: { ...settings.docker, unused_images_enabled: enabled },
              })
            }
            aria-label="Clean unused Docker images"
            data-testid="storage-docker-unused-images"
          />
        }
      />
      <DockerThresholdFields settings={settings} pending={pending} onChange={onChange} />
      {disabledReason && <p className="text-xs text-muted-foreground">{disabledReason}</p>}
    </div>
  );
}

export function StoragePolicyCard({
  settings,
  capabilities,
  pending,
  onChange,
  onSave,
  onDedicatedConfirm,
  onAdopt,
}: Props) {
  const [dockerDialogOpen, setDockerDialogOpen] = useState(false);
  const [adoptionDialogOpen, setAdoptionDialogOpen] = useState(false);
  const [adoptionPath, setAdoptionPath] = useState("");
  return (
    <Card className="min-w-0" data-testid="storage-policy-card">
      <CardHeader>
        <CardTitle className="text-base">Maintenance policy</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <CorePolicySection
          settings={settings}
          capabilities={capabilities}
          pending={pending}
          onChange={onChange}
        />
        {capabilities.go_cache_adoption_available && (
          <AdoptionSection
            path={adoptionPath}
            setPath={setAdoptionPath}
            pending={pending}
            onOpen={() => setAdoptionDialogOpen(true)}
          />
        )}
        <DockerPolicySection
          settings={settings}
          capabilities={capabilities}
          pending={pending}
          onChange={onChange}
          onOpen={() => setDockerDialogOpen(true)}
        />
        <div className="flex justify-end">
          <StorageActionButton
            disabledReason={pending ? "Wait for the current storage action to finish." : undefined}
            onClick={() => void onSave()}
            data-testid="storage-save-settings"
          >
            Save policy
          </StorageActionButton>
        </div>
      </CardContent>
      <DedicatedDockerDialog
        open={dockerDialogOpen}
        onOpenChange={setDockerDialogOpen}
        onConfirm={() => {
          const next = settingsWithDockerAcknowledgement(settings, true);
          onChange(next);
          void onDedicatedConfirm(next);
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
    </Card>
  );
}
