"use client";

import { Separator } from "@kandev/ui/separator";
import { useTranslation } from "react-i18next";
import { SettingsTarget } from "@/components/settings/settings-target";
import { AboutCard } from "@/components/settings/system/about-card";
import { LicensesList } from "@/components/settings/system/licenses-list";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";
import type { LicenseEntry } from "@/lib/types/system";

/** About: the former About and Licenses pages as one page. */
export function AboutSettings({ licenses }: { licenses: LicenseEntry[] }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <AboutCard />
      <Separator />
      <SettingsTarget targetId={SYSTEM_SETTINGS_TARGETS.licenses} className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold">{t("system:navLicenses")}</h3>
          <p className="text-sm text-muted-foreground mt-1">
            {t("system:licensesPageDescription")}
          </p>
        </div>
        <LicensesList entries={licenses} />
      </SettingsTarget>
    </div>
  );
}
