"use client";

import { Separator } from "@kandev/ui/separator";
import { useTranslation } from "react-i18next";
import { SettingsGroup } from "@/components/settings/settings-group";
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
      <SettingsGroup
        title={t("system:navLicenses")}
        description={t("system:licensesPageDescription")}
        discoveryTargetId={SYSTEM_SETTINGS_TARGETS.licenses}
        contentClassName="divide-y-0"
      >
        <LicensesList entries={licenses} />
      </SettingsGroup>
    </div>
  );
}
