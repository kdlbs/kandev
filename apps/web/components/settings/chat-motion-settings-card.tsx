"use client";

import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsCard } from "./settings-card";

export function ChatMotionSettingsCard({
  enabled,
  isDirty,
  onChange,
}: {
  enabled: boolean;
  isDirty: boolean;
  onChange: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.chatMotion}
      data-testid="chat-motion-settings-card"
    >
      <CardContent>
        <div
          className="flex items-center justify-between gap-4"
          data-testid="chat-motion-toggle-row"
        >
          <div className="min-w-0 space-y-1">
            <Label htmlFor="animate-chat">{t("settings:chatAnimations")}</Label>
            <p id="chat-motion-description" className="max-w-3xl text-xs text-muted-foreground">
              {t("settings:chatAnimationsDescription")}
            </p>
          </div>
          <Switch
            id="animate-chat"
            checked={enabled}
            onCheckedChange={onChange}
            aria-describedby="chat-motion-description"
            data-settings-dirty={isDirty}
            className="data-[state=checked]:bg-transparent data-[state=unchecked]:bg-transparent dark:data-[state=unchecked]:bg-transparent data-[size=default]:h-7 data-[size=default]:w-9 [@media(pointer:coarse)]:data-[size=default]:h-11 [@media(pointer:coarse)]:data-[size=default]:w-11 cursor-pointer shrink-0 p-1 [@media(pointer:coarse)]:p-2 before:absolute before:left-1 [@media(pointer:coarse)]:before:left-2 before:top-1/2 before:h-[16.6px] before:w-7 before:-translate-y-1/2 before:rounded-full before:bg-input before:content-[''] data-[state=checked]:before:bg-primary dark:data-[state=unchecked]:before:bg-input/80 [&_[data-slot=switch-thumb]]:z-10"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
