"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";

export function PersonaExecutorField({
  value,
  onChange,
}: {
  value: string;
  onChange: (profileId: string, type: string) => void;
}) {
  const { t } = useTranslation();
  const executors = useAppStore((s) => s.executors.items);
  const profiles = executors.flatMap((executor) =>
    (executor.profiles ?? []).map((profile) => ({
      ...profile,
      type: executor.type,
      host: executor.name,
    })),
  );
  return (
    <div className="space-y-2">
      <Label>{t("orchestration:personaExecutorProfile")}</Label>
      <Select
        value={value || "__inherit__"}
        onValueChange={(id) => {
          const profile = profiles.find((item) => item.id === id);
          onChange(profile?.id ?? "", profile?.type ?? "");
        }}
      >
        <SelectTrigger data-testid="persona-executor-profile">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__inherit__">{t("orchestration:inherit")}</SelectItem>
          {value && !profiles.some((profile) => profile.id === value) && (
            <SelectItem value={value}>{value}</SelectItem>
          )}
          {profiles.map((profile) => (
            <SelectItem key={profile.id} value={profile.id}>
              {profile.host}: {profile.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-sm text-muted-foreground">
        {t("orchestration:personaExecutorProfileHint")}
      </p>
    </div>
  );
}
