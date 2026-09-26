"use client";

import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";

type Props = {
  enabled: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
};

export function RoutingEnableCard({ enabled, onChange, disabled }: Props) {
  const { t } = useTranslation();
  return (
    <div className="rounded-lg border border-border p-4 space-y-3">
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium">{t("office:automaticProviderFallback")}</p>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("office:whenEnabledProviderLimitsCanMove")}
          </p>
        </div>
        <Switch
          checked={enabled}
          onCheckedChange={onChange}
          disabled={disabled}
          className="cursor-pointer"
        />
      </div>
    </div>
  );
}
