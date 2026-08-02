"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLanguage } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsSection } from "@/components/settings/settings-section";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { readBootPayload } from "@/src/boot-payload";
import { pseudoLocaleAvailable } from "@/lib/i18n/boot";
import {
  activateLocale,
  i18n,
  LOCALE_LABELS,
  normalizeLocale,
  selectableLocales,
  type SupportedLocale,
} from "@/lib/i18n";

/**
 * Language switcher. Applies immediately (no deferred save) — `activateLocale`
 * re-renders the app and persists the choice to the `kandev_locale` cookie — so
 * this surface owns no unsaved state and does not register a save contributor.
 * The pseudo QA locale is hidden in production builds.
 */
export function LanguageSettings() {
  const { t } = useTranslation();
  const [locale, setLocale] = useState<SupportedLocale>(() => normalizeLocale(i18n.language));

  // Not `import.meta.env.PROD`: the e2e harness serves a production bundle, so
  // that constant would hide the pseudo locale the QA specs rely on.
  const options = selectableLocales(!pseudoLocaleAvailable(readBootPayload()));

  const handleChange = async (value: string) => {
    const activated = await activateLocale(value);
    setLocale(activated);
  };

  return (
    <SettingsSection
      icon={<IconLanguage className="h-5 w-5" />}
      title={t("settings:language")}
      description={t("settings:chooseTheLanguageUsedAcrossThe")}
    >
      <SettingsCard>
        {/* Card only supplies vertical padding; the horizontal px-4 comes from
            CardContent, which every other settings card here goes through. */}
        <CardContent>
          <div className="flex flex-col gap-2">
            <Label htmlFor="language-select">{t("settings:displayLanguage")}</Label>
            <Select value={locale} onValueChange={handleChange}>
              <SelectTrigger
                id="language-select"
                className="w-64"
                aria-label={t("settings:displayLanguage")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {options.map((candidate) => (
                  <SelectItem key={candidate} value={candidate}>
                    {LOCALE_LABELS[candidate]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-sm text-muted-foreground">
              {t("settings:changesTheLanguageOfTheKandev")}
            </p>
          </div>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
