"use client";

import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useTranslation } from "react-i18next";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";

type DockerStorageLimitsCardProps = {
  dockerTmpSize: string;
  setDockerTmpSize: (value: string) => void;
  baselineDockerTmpSize: string;
  dockerLogMaxSize: string;
  setDockerLogMaxSize: (value: string) => void;
  baselineDockerLogMaxSize: string;
  dockerLogMaxFiles: string;
  setDockerLogMaxFiles: (value: string) => void;
  baselineDockerLogMaxFiles: string;
};

export function DockerStorageLimitsCard({
  dockerTmpSize,
  setDockerTmpSize,
  baselineDockerTmpSize,
  dockerLogMaxSize,
  setDockerLogMaxSize,
  baselineDockerLogMaxSize,
  dockerLogMaxFiles,
  setDockerLogMaxFiles,
  baselineDockerLogMaxFiles,
}: DockerStorageLimitsCardProps) {
  const { t } = useTranslation();
  const isDirty =
    dockerTmpSize !== baselineDockerTmpSize ||
    dockerLogMaxSize !== baselineDockerLogMaxSize ||
    dockerLogMaxFiles !== baselineDockerLogMaxFiles;

  return (
    <SettingsCard isDirty={isDirty}>
      <SettingsCardHeader
        title={t("executors:dockerStorageLimitsTitle")}
        description={t("executors:dockerStorageLimitsDescription")}
      />
      <CardContent className="space-y-4">
        <StorageLimitField
          id="docker-tmp-size"
          label={t("executors:dockerTmpSizeLabel")}
          placeholder={t("executors:dockerTmpSizePlaceholder")}
          value={dockerTmpSize}
          onChange={setDockerTmpSize}
        />
        <StorageLimitField
          id="docker-log-max-size"
          label={t("executors:dockerLogMaxSizeLabel")}
          placeholder={t("executors:dockerLogMaxSizePlaceholder")}
          value={dockerLogMaxSize}
          onChange={setDockerLogMaxSize}
        />
        <StorageLimitField
          id="docker-log-max-files"
          label={t("executors:dockerLogMaxFilesLabel")}
          placeholder={t("executors:dockerLogMaxFilesPlaceholder")}
          value={dockerLogMaxFiles}
          onChange={setDockerLogMaxFiles}
        />
        <p className="text-sm text-muted-foreground">{t("executors:dockerStorageDefaultsNote")}</p>
      </CardContent>
    </SettingsCard>
  );
}

function StorageLimitField({
  id,
  label,
  placeholder,
  value,
  onChange,
}: {
  id: string;
  label: string;
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="grid gap-2 md:grid-cols-[minmax(12rem,1fr)_minmax(0,2fr)] md:items-center">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="font-mono"
      />
    </div>
  );
}
