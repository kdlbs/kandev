"use client";

import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { SettingsCard } from "@/components/settings/settings-card";

type ProfileDetailsCardProps = {
  name: string;
  baselineName?: string;
  onNameChange: (v: string) => void;
};

export function ProfileDetailsCard({ name, baselineName, onNameChange }: ProfileDetailsCardProps) {
  const isDirty = baselineName !== undefined && name.trim() !== baselineName.trim();
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle>Profile Details</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="profile-name">Name</Label>
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
