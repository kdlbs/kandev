"use client";

import { Trans, useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";

// An example sandbox launcher invocation — a shell fragment the user types
// verbatim, so it is interpolated as a value rather than written into the
// catalog.
const EXAMPLE_PREFIX = "greywall --";

export function CommandPrefixField({
  profile,
  baselineProfile,
  onChange,
}: {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-2"
      data-settings-dirty={
        Boolean(baselineProfile) &&
        (profile.command_prefix ?? "") !== (baselineProfile?.command_prefix ?? "")
      }
      data-settings-dirty-level="container"
    >
      <Label htmlFor="profile-command-prefix">{t("agents:commandPrefix")}</Label>
      <Input
        id="profile-command-prefix"
        data-testid="command-prefix-input"
        value={profile.command_prefix ?? ""}
        onChange={(event) => onChange({ command_prefix: event.target.value })}
        placeholder={t("agents:exampleValue", { example: EXAMPLE_PREFIX })}
      />
      <p className="text-xs text-muted-foreground">
        <Trans i18nKey="agents:commandPrefixHelp" values={{ example: EXAMPLE_PREFIX }}>
          <code />
        </Trans>
      </p>
    </div>
  );
}
