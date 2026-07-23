"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";

export function CommandPrefixField({
  profile,
  baselineProfile,
  onChange,
}: {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  return (
    <div
      className="space-y-2"
      data-settings-dirty={
        Boolean(baselineProfile) &&
        (profile.command_prefix ?? "") !== (baselineProfile?.command_prefix ?? "")
      }
      data-settings-dirty-level="container"
    >
      <Label htmlFor="profile-command-prefix">Command prefix</Label>
      <Input
        id="profile-command-prefix"
        data-testid="command-prefix-input"
        value={profile.command_prefix ?? ""}
        onChange={(event) => onChange({ command_prefix: event.target.value })}
        placeholder="e.g. greywall --"
      />
      <p className="text-xs text-muted-foreground">
        Tokens prepended to the agent launch command, so it runs under a sandbox launcher (e.g.{" "}
        <code>greywall --</code>). The value is shell-tokenised. Leave empty to run the agent
        directly.
      </p>
    </div>
  );
}
