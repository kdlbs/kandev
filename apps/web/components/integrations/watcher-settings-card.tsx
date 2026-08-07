"use client";

import type { ReactNode } from "react";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { useTranslation } from "react-i18next";

type WatcherSettingsCardProps = {
  children: ReactNode;
  isDirty: boolean;
  isLoading: boolean;
  isEmpty: boolean;
  testId: string;
};

export function WatcherSettingsCard({
  children,
  isDirty,
  isLoading,
  isEmpty,
  testId,
}: WatcherSettingsCardProps) {
  const { t } = useTranslation();
  return (
    <SettingsCard isDirty={isDirty} data-testid={testId}>
      <CardContent className="pt-6">
        {isLoading && isEmpty ? (
          <p className="py-4 text-center text-sm text-muted-foreground">
            {t("integrations:loading")}
          </p>
        ) : (
          children
        )}
      </CardContent>
    </SettingsCard>
  );
}
