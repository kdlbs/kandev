"use client";

import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { useTranslation } from "react-i18next";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";

type GitIdentityCardProps = {
  name: string;
  email: string;
  onNameChange: (value: string) => void;
  onEmailChange: (value: string) => void;
};

export function GitIdentityCard({
  name,
  email,
  onNameChange,
  onEmailChange,
}: GitIdentityCardProps) {
  const { t } = useTranslation();
  return (
    <Card>
      <SettingsCardHeader
        title={t("executors:gitIdentity")}
        description={t("executors:optionalAuthorIdentityAppliedInRemote")}
      />
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <SettingsFieldLabel htmlFor="git-user-name">
            {t("executors:gitUserName")}
          </SettingsFieldLabel>
          <Input
            id="git-user-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            placeholder={t("executors:janeDeveloper")}
          />
        </div>
        <div className="space-y-2">
          <SettingsFieldLabel htmlFor="git-user-email">
            {t("executors:gitUserEmail")}
          </SettingsFieldLabel>
          <Input
            id="git-user-email"
            value={email}
            onChange={(e) => onEmailChange(e.target.value)}
            placeholder="jane@example.com"
          />
        </div>
      </CardContent>
    </Card>
  );
}
