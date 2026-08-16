"use client";

import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
import { useTranslation } from "react-i18next";

type ProfileDetailsCardProps = {
  name: string;
  baselineName?: string;
  onNameChange: (v: string) => void;
  discoveryTargetId?: string;
};

export function ProfileDetailsCard({
  name,
  baselineName,
  onNameChange,
  discoveryTargetId,
}: ProfileDetailsCardProps) {
  const { t } = useTranslation();
  const isDirty = baselineName !== undefined && name.trim() !== baselineName.trim();
  return (
    <SettingsCard isDirty={isDirty} discoveryTargetId={discoveryTargetId}>
      <SettingsCardHeader title={t("executors:profileDetails")} />
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <SettingsFieldLabel htmlFor="profile-name">{t("executors:name")}</SettingsFieldLabel>
          <Input
            id="profile-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            data-settings-dirty={isDirty}
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
